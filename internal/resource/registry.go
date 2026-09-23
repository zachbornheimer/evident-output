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
}

// Registry grants claims. The zero value is not usable; call NewRegistry.
type Registry struct {
	onContended func(Claim)

	mu sync.Mutex
	// held are the claims currently granted.
	held []*hold
	// queue holds waiting claims in arrival order. A claim is granted only
	// when it conflicts with nothing held and nothing queued ahead of it,
	// which keeps a writer from starving behind a stream of readers.
	queue []*hold
	// changed is closed (and replaced) whenever held or queue shrinks, to
	// wake every waiter so it can re-check its turn.
	changed chan struct{}
}

// hold is one granted or waiting claim. released is atomic because a
// nested-acquisition check may read a hold owned by another Registry.
type hold struct {
	claim    Claim
	released atomic.Bool
}

// Option configures a Registry.
type Option func(*Registry)

// OnContended registers fn to run, at most once per Hold and outside any
// registry lock, when a claim must wait for a conflicting one. An
// uncontended claim never calls it, so quiet acquisitions stay invisible.
func OnContended(fn func(Claim)) Option {
	return func(r *Registry) { r.onContended = fn }
}

// NewRegistry returns an empty Registry.
func NewRegistry(opts ...Option) *Registry {
	r := &Registry{changed: make(chan struct{})}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

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

// HoldResource resolves req and holds it for fn, exactly like Hold. The
// nested-acquisition check runs before resolution so misuse is reported as
// ErrNested regardless of whether the requested Resource resolves.
func (r *Registry) HoldResource(ctx context.Context, req Request, fn func(context.Context) error) error {
	if h, ok := holding(ctx); ok {
		return nestedError(h, fmt.Sprintf("%s %v", req.Mode, req.Resource))
	}
	key, err := Resolve(req.Resource, req.Workspace)
	if err != nil {
		return fmt.Errorf("evo: hold %s %v: %w", req.Mode, req.Resource, err)
	}
	return r.Hold(ctx, Claim{Key: key, Mode: req.Mode}, fn)
}

// Hold waits until c can be granted, runs fn with a context derived from
// ctx that records the claim, and releases the claim when fn returns or
// panics. ctx must be non-nil.
//
// If ctx already holds a claim (directly, or through any helper it was
// passed to), Hold returns ErrNested immediately. If ctx ends while
// waiting, Hold returns its cause and fn never runs.
func (r *Registry) Hold(ctx context.Context, c Claim, fn func(context.Context) error) error {
	if h, ok := holding(ctx); ok {
		return nestedError(h, c.String())
	}
	h, cause := r.acquire(ctx, c)
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
func (r *Registry) acquire(ctx context.Context, c Claim) (*hold, error) {
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	h := &hold{claim: c}
	reported := false
	r.mu.Lock()
	r.queue = append(r.queue, h)
	for {
		if r.grantableLocked(h) {
			r.dequeueLocked(h)
			r.held = append(r.held, h)
			r.mu.Unlock()
			return h, nil
		}
		changed := r.changed
		r.mu.Unlock()
		if !reported && r.onContended != nil {
			reported = true
			r.onContended(c)
		}
		select {
		case <-changed:
			r.mu.Lock()
		case <-ctx.Done():
			r.mu.Lock()
			r.dequeueLocked(h)
			r.broadcastLocked()
			r.mu.Unlock()
			return nil, context.Cause(ctx)
		}
	}
}

// grantableLocked reports whether h conflicts with nothing held and
// nothing queued ahead of it.
func (r *Registry) grantableLocked(h *hold) bool {
	for _, g := range r.held {
		if g.claim.conflicts(h.claim) {
			return false
		}
	}
	for _, q := range r.queue {
		if q == h {
			return true
		}
		if q.claim.conflicts(h.claim) {
			return false
		}
	}
	return true
}

func (r *Registry) dequeueLocked(h *hold) {
	r.queue = slices.DeleteFunc(r.queue, func(q *hold) bool { return q == h })
}

func (r *Registry) release(h *hold) {
	h.released.Store(true)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.held = slices.DeleteFunc(r.held, func(g *hold) bool { return g == h })
	r.broadcastLocked()
}

func (r *Registry) broadcastLocked() {
	close(r.changed)
	r.changed = make(chan struct{})
}

// occupancy reports how many claims are held and queued; tests use it to
// prove no path leaks ownership.
func (r *Registry) occupancy() (held, queued int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.held), len(r.queue)
}
