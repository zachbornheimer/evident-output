package resource

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
)

// Mode is how a claim touches its resource.
type Mode uint8

const (
	// Read observes the resource; any number of overlapping reads share it.
	Read Mode = iota
	// Write changes the resource; it excludes every overlapping claim.
	Write
)

// String renders the mode as "read" or "write".
func (m Mode) String() string {
	if m == Write {
		return "write"
	}
	return "read"
}

// Claim is one request for a resolved Key in a Mode.
type Claim struct {
	Key  Key
	Mode Mode
}

// String renders the claim as "<mode> <key>", e.g. "write fs:/repo".
func (c Claim) String() string { return c.Mode.String() + " " + c.Key.String() }

// conflicts reports whether c and o may not be held at the same time:
// their keys overlap and at least one of them writes.
func (c Claim) conflicts(o Claim) bool {
	return (c.Mode == Write || o.Mode == Write) && c.Key.overlaps(o.Key)
}

// ErrNested is returned, without waiting, when a context that already
// holds a resource asks for another one. Holding at most one resource at a
// time is what rules deadlock out, so a second acquisition is misuse even
// when it would not conflict.
var ErrNested = errors.New("evo: nested resource acquisition")

// Request is an unresolved claim: a Resource, the workspace a relative
// filesystem path resolves against, and the access Mode.
type Request struct {
	Resource  Resource
	Workspace string
	Mode      Mode
	// OnContended, when non-nil, runs once, outside any registry lock, and
	// only if the claim has to wait. It lets one process-wide Registry
	// report contention to whichever caller is actually waiting.
	OnContended func(Claim)
}

// Registry grants claims. The zero value is not usable; call NewRegistry.
type Registry struct {
	mu sync.Mutex
	// held are the claims currently granted.
	held []*hold
	// queue holds waiting claims in arrival order. A claim is granted only
	// when it conflicts with nothing held and nothing queued ahead of it,
	// which keeps a writer from starving behind a stream of readers.
	queue []*hold
}

// hold is one granted or waiting claim. released is atomic because a
// nested-acquisition check may read a hold owned by another Registry.
// ready is closed exactly once, when the claim moves from queue to held, so
// a release wakes only the claims it actually grants.
type hold struct {
	claim    Claim
	released atomic.Bool
	ready    chan struct{}
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry { return &Registry{} }

// heldKey is the context key under which Hold records the claim its
// callback owns — unexported so nothing outside this package can forge or
// clear it.
type heldKey struct{}

// holding returns the live claim ctx carries, if any. A claim released
// before ctx is reused (a context captured in a callback) no longer counts.
func holding(ctx context.Context) (*hold, bool) {
	h, ok := ctx.Value(heldKey{}).(*hold)
	if !ok || h.released.Load() {
		return nil, false
	}
	return h, true
}

// HoldResource resolves req, waits until its claim can be granted, runs fn
// with a context derived from ctx that records the claim, and releases the
// claim when fn returns or panics. ctx must be non-nil.
//
// If ctx already holds a claim (directly, or through any helper it was
// passed to), HoldResource returns ErrNested immediately — before
// resolution, so misuse is reported even for a Resource that would not
// resolve. If ctx ends while waiting, it returns its cause and fn never
// runs.
func (r *Registry) HoldResource(ctx context.Context, req Request, fn func(context.Context) error) error {
	if err := CheckFree(ctx, fmt.Sprintf("%s %v", req.Mode, req.Resource)); err != nil {
		return err
	}
	key, err := Resolve(req.Resource, req.Workspace)
	if err != nil {
		return fmt.Errorf("evo: hold %s %v: %w", req.Mode, req.Resource, err)
	}
	return r.hold(ctx, Claim{Key: key, Mode: req.Mode}, req.OnContended, fn)
}

// CheckFree returns ErrNested, naming requested, when ctx already holds a
// resource. Operations that must not block while a claim is held (opening
// a cross-process manifest lock, for one) call it before doing anything
// that could wait.
func CheckFree(ctx context.Context, requested string) error {
	if h, ok := holding(ctx); ok {
		return nestedError(h, requested)
	}
	return nil
}

// hold acquires c, runs fn under it, and releases it. The caller has
// already refused a nested claim (CheckFree).
func (r *Registry) hold(ctx context.Context, c Claim, onContended func(Claim), fn func(context.Context) error) error {
	h, cause := r.acquire(ctx, c, onContended)
	if h == nil {
		return fmt.Errorf("evo: acquire %s: %w", c, cause)
	}
	defer r.release(h)
	return fn(context.WithValue(ctx, heldKey{}, h))
}

func nestedError(h *hold, requested string) error {
	return fmt.Errorf("%w: holding %s, requested %s", ErrNested, h.claim, requested)
}

// acquire queues c and blocks until it is granted, or returns a nil hold
// and ctx's cancellation cause once ctx ends first.
func (r *Registry) acquire(ctx context.Context, c Claim, onContended func(Claim)) (*hold, error) {
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	h := &hold{claim: c, ready: make(chan struct{})}
	r.mu.Lock()
	r.queue = append(r.queue, h)
	r.grantLocked()
	r.mu.Unlock()
	select {
	case <-h.ready:
		return h, nil
	default:
	}
	if onContended != nil {
		onContended(c)
	}
	select {
	case <-h.ready:
		return h, nil
	case <-ctx.Done():
		r.abandon(h)
		return nil, context.Cause(ctx)
	}
}

// abandon withdraws h after its context ended: out of the queue if it is
// still waiting, or out of held if it was granted in the same instant.
// Either way the claims it was blocking get their turn.
func (r *Registry) abandon(h *hold) {
	h.released.Store(true)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queue = slices.DeleteFunc(r.queue, func(q *hold) bool { return q == h })
	r.held = slices.DeleteFunc(r.held, func(g *hold) bool { return g == h })
	r.grantLocked()
}

// grantLocked walks the queue once, in arrival order, and grants every
// claim that conflicts with nothing held and nothing still queued ahead of
// it, closing only those claims' ready channels.
func (r *Registry) grantLocked() {
	waiting := r.queue[:0]
	for _, q := range r.queue {
		if r.conflictsLocked(q, waiting) {
			waiting = append(waiting, q)
			continue
		}
		r.held = append(r.held, q)
		close(q.ready)
	}
	clear(r.queue[len(waiting):])
	r.queue = waiting
}

// conflictsLocked reports whether h conflicts with a held claim or with a
// claim still waiting ahead of it.
func (r *Registry) conflictsLocked(h *hold, ahead []*hold) bool {
	for _, g := range r.held {
		if g.claim.conflicts(h.claim) {
			return true
		}
	}
	for _, q := range ahead {
		if q.claim.conflicts(h.claim) {
			return true
		}
	}
	return false
}

func (r *Registry) release(h *hold) {
	h.released.Store(true)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.held = slices.DeleteFunc(r.held, func(g *hold) bool { return g == h })
	r.grantLocked()
}

// occupancy reports how many claims are held and queued; tests use it to
// prove no path leaks ownership.
func (r *Registry) occupancy() (held, queued int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.held), len(r.queue)
}
