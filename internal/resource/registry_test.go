package resource

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// deadlockBudget bounds every wait in these tests: a correct registry
// finishes each step in microseconds, so hitting it means a real hang.
const deadlockBudget = 5 * time.Second

// holder runs one Hold on its own goroutine and keeps the claim until
// released, so a test can arrange overlapping holders deterministically.
type holder struct {
	entered chan struct{}
	release chan struct{}
	done    chan error
}

func startHolder(ctx context.Context, r *Registry, c Claim) *holder {
	h := &holder{entered: make(chan struct{}), release: make(chan struct{}), done: make(chan error, 1)}
	go func() {
		h.done <- r.Hold(ctx, c, func(context.Context) error {
			close(h.entered)
			<-h.release
			return nil
		})
	}()
	return h
}

func (h *holder) waitEntered(t *testing.T) {
	t.Helper()
	select {
	case <-h.entered:
	case <-time.After(deadlockBudget):
		t.Fatal("holder never entered its claim")
	}
}

func (h *holder) finish(t *testing.T) {
	t.Helper()
	close(h.release)
	select {
	case err := <-h.done:
		if err != nil {
			t.Fatalf("holder Hold returned %v", err)
		}
	case <-time.After(deadlockBudget):
		t.Fatal("holder never returned")
	}
}

func (h *holder) hasEntered() bool {
	select {
	case <-h.entered:
		return true
	default:
		return false
	}
}

// contendedSignal returns a registry whose OnContended hook reports each
// claim that had to wait, so tests observe "blocked" without sleeping.
func contendedSignal() (*Registry, <-chan Claim) {
	ch := make(chan Claim, 16)
	return newObservedRegistry(func(c Claim) { ch <- c }), ch
}

func awaitContended(t *testing.T, ch <-chan Claim, want Claim) {
	t.Helper()
	select {
	case got := <-ch:
		if got != want {
			t.Fatalf("contended claim = %v, want %v", got, want)
		}
	case <-time.After(deadlockBudget):
		t.Fatalf("claim %v never reported contention", want)
	}
}

func assertIdle(t *testing.T, r *Registry) {
	t.Helper()
	held, queued := r.occupancy()
	if held != 0 || queued != 0 {
		t.Fatalf("registry leaked ownership: held=%d queued=%d", held, queued)
	}
}

func TestHoldCompatibleClaimsOverlap(t *testing.T) {
	cases := []struct {
		name  string
		first Claim
		other Claim
	}{
		{"read/read same file", readOf(fsKey("/repo/a.go")), readOf(fsKey("/repo/a.go"))},
		{"read/read ancestor", readOf(fsKey("/repo")), readOf(fsKey("/repo/a.go"))},
		{"write/write siblings", writeOf(fsKey("/repo/a.go")), writeOf(fsKey("/repo/b.go"))},
		{"write/write distinct logical", writeOf(logicalKey("brew")), writeOf(logicalKey("apt"))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newObservedRegistry(func(c Claim) { t.Errorf("compatible claim %v was made to wait", c) })
			first := startHolder(context.Background(), r, tc.first)
			first.waitEntered(t)
			second := startHolder(context.Background(), r, tc.other)
			second.waitEntered(t) // both inside at once: overlap proven
			first.finish(t)
			second.finish(t)
			assertIdle(t, r)
		})
	}
}

func TestHoldConflictingClaimsExclude(t *testing.T) {
	cases := []struct {
		name  string
		first Claim
		other Claim
	}{
		{"read then write", readOf(fsKey("/repo/a.go")), writeOf(fsKey("/repo/a.go"))},
		{"write then read", writeOf(fsKey("/repo/a.go")), readOf(fsKey("/repo/a.go"))},
		{"write then write", writeOf(fsKey("/repo/a.go")), writeOf(fsKey("/repo/a.go"))},
		{"coarse write excludes child read", writeOf(fsKey("/repo")), readOf(fsKey("/repo/pkg/a.go"))},
		{"child write excludes coarse read", writeOf(fsKey("/repo/pkg/a.go")), readOf(fsKey("/repo"))},
		{"same logical name", writeOf(logicalKey("brew")), writeOf(logicalKey("brew"))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, contended := contendedSignal()
			first := startHolder(context.Background(), r, tc.first)
			first.waitEntered(t)
			second := startHolder(context.Background(), r, tc.other)
			awaitContended(t, contended, tc.other)
			if second.hasEntered() {
				t.Fatal("conflicting claim entered while the first was still held")
			}
			first.finish(t)
			second.waitEntered(t)
			second.finish(t)
			assertIdle(t, r)
		})
	}
}

// TestHoldWriteExclusionUnderRace drives many goroutines through a
// non-atomic read-modify-write guarded only by overlapping write claims.
// Under -race any lapse in exclusion is a reported data race, and the
// final count is exact only if no two writers ever overlapped.
func TestHoldWriteExclusionUnderRace(t *testing.T) {
	r := NewRegistry()
	// Every pair overlaps: each key is an ancestor or descendant of the others.
	keys := []Key{fsKey("/repo"), fsKey("/repo/pkg"), fsKey("/repo/pkg/b.go")}
	const workers, rounds = 8, 50
	counter := 0
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := range rounds {
				c := writeOf(keys[(w+i)%len(keys)])
				if err := r.Hold(context.Background(), c, func(context.Context) error {
					counter++
					return nil
				}); err != nil {
					t.Errorf("Hold(%v): %v", c, err)
					return
				}
			}
		})
	}
	wg.Wait()
	if counter != workers*rounds {
		t.Fatalf("counter = %d, want %d: overlapping writers raced", counter, workers*rounds)
	}
	assertIdle(t, r)
}

// TestHoldReadersShareUnderRace proves concurrent read claims coexist with
// writers correctly: readers only read the value, writers mutate it, and
// the race detector flags any reader that overlapped a writer.
func TestHoldReadersShareUnderRace(t *testing.T) {
	r := NewRegistry()
	k := fsKey("/repo/state.json")
	value := 0
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			mode := Read
			if i%4 == 0 {
				mode = Write
			}
			_ = r.Hold(context.Background(), claim(k, mode), func(context.Context) error {
				if mode == Write {
					value++
				} else {
					_ = value
				}
				return nil
			})
		})
	}
	wg.Wait()
	if value != 4 {
		t.Fatalf("value = %d, want 4", value)
	}
	assertIdle(t, r)
}

// A steady stream of readers must not starve a waiting writer: a reader
// that arrives after a queued conflicting writer waits behind it.
func TestHoldQueuedWriterIsNotStarvedByLaterReaders(t *testing.T) {
	r, contended := contendedSignal()
	k := fsKey("/repo/a.go")
	reader := startHolder(context.Background(), r, readOf(k))
	reader.waitEntered(t)
	writer := startHolder(context.Background(), r, writeOf(k))
	awaitContended(t, contended, writeOf(k))
	late := startHolder(context.Background(), r, readOf(k))
	awaitContended(t, contended, readOf(k))
	reader.finish(t)
	writer.waitEntered(t)
	if late.hasEntered() {
		t.Fatal("late reader overtook the queued writer")
	}
	writer.finish(t)
	late.waitEntered(t)
	late.finish(t)
	assertIdle(t, r)
}

func TestHoldNestedAcquisitionFailsDirectly(t *testing.T) {
	r := NewRegistry()
	outer := writeOf(fsKey("/repo/a.go"))
	for _, inner := range []Claim{readOf(fsKey("/other")), outer, writeOf(logicalKey("brew"))} {
		t.Run(inner.String(), func(t *testing.T) {
			ran := false
			err := r.Hold(context.Background(), outer, func(ctx context.Context) error {
				innerErr := r.Hold(ctx, inner, func(context.Context) error {
					ran = true
					return nil
				})
				if !errors.Is(innerErr, ErrNested) {
					t.Errorf("nested Hold error = %v, want ErrNested", innerErr)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("outer Hold: %v", err)
			}
			if ran {
				t.Fatal("nested callback ran")
			}
			assertIdle(t, r)
		})
	}
}

// saveReport is a helper a caller might write without knowing it acquires
// a resource; its caller already holds one.
func saveReport(ctx context.Context, r *Registry) error {
	return r.Hold(ctx, writeOf(fsKey("/repo/report.txt")), func(context.Context) error { return nil })
}

// helperKey is an unrelated context value a helper layers on top of the
// held context; it must not hide the held claim.
type helperKey struct{}

func TestHoldNestedAcquisitionFailsThroughHelpers(t *testing.T) {
	r := NewRegistry()
	err := r.Hold(context.Background(), writeOf(logicalKey("db")), func(ctx context.Context) error {
		derived, cancel := context.WithTimeout(context.WithValue(ctx, helperKey{}, "report"), time.Minute)
		defer cancel()
		return saveReport(derived, r)
	})
	if !errors.Is(err, ErrNested) {
		t.Fatalf("Hold error = %v, want ErrNested propagated from the helper", err)
	}
	assertIdle(t, r)
}

// Nesting is misuse across registries too: the invariant is "one resource
// held", not "one resource per registry".
func TestHoldNestedAcquisitionFailsAcrossRegistries(t *testing.T) {
	a, b := NewRegistry(), NewRegistry()
	err := a.Hold(context.Background(), readOf(logicalKey("x")), func(ctx context.Context) error {
		return b.Hold(ctx, readOf(logicalKey("y")), func(context.Context) error { return nil })
	})
	if !errors.Is(err, ErrNested) {
		t.Fatalf("Hold error = %v, want ErrNested", err)
	}
}

// The classic ABBA deadlock: each holder tries to take the resource the
// other holds. Both must fail fast as misuse instead of waiting forever,
// even though the requested resource is contended.
func TestHoldNestedAcquisitionFailsBeforeDeadlock(t *testing.T) {
	r := NewRegistry()
	a, b := writeOf(fsKey("/a")), writeOf(fsKey("/b"))
	bothHeld := sync.WaitGroup{}
	bothHeld.Add(2)
	errs := make(chan error, 2)
	for _, pair := range [][2]Claim{{a, b}, {b, a}} {
		go func() {
			errs <- r.Hold(context.Background(), pair[0], func(ctx context.Context) error {
				bothHeld.Done()
				bothHeld.Wait()
				return r.Hold(ctx, pair[1], func(context.Context) error { return nil })
			})
		}()
	}
	for range 2 {
		select {
		case err := <-errs:
			if !errors.Is(err, ErrNested) {
				t.Fatalf("Hold error = %v, want ErrNested", err)
			}
		case <-time.After(deadlockBudget):
			t.Fatal("nested acquisition deadlocked instead of failing")
		}
	}
	assertIdle(t, r)
}

func TestHoldNestedErrorNamesBothClaims(t *testing.T) {
	r := NewRegistry()
	err := r.Hold(context.Background(), writeOf(fsKey("/repo")), func(ctx context.Context) error {
		return r.Hold(ctx, readOf(logicalKey("brew")), func(context.Context) error { return nil })
	})
	const want = "evo: nested resource acquisition: holding write fs:/repo, requested read logical:brew"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

// A context captured during a hold and reused after release carries no
// live claim, so it may acquire again.
func TestHoldCapturedContextAfterReleaseIsNotNested(t *testing.T) {
	r := NewRegistry()
	var captured context.Context
	if err := r.Hold(context.Background(), writeOf(logicalKey("a")), func(ctx context.Context) error {
		captured = ctx
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.Hold(captured, writeOf(logicalKey("b")), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Hold with released captured ctx: %v", err)
	}
	assertIdle(t, r)
}

func TestHoldReleasesAfterCallbackError(t *testing.T) {
	r := NewRegistry()
	boom := errors.New("boom")
	c := writeOf(fsKey("/repo/a.go"))
	if err := r.Hold(context.Background(), c, func(context.Context) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("Hold error = %v, want %v", err, boom)
	}
	assertIdle(t, r)
}

func TestHoldReleasesAfterCallbackPanic(t *testing.T) {
	r := NewRegistry()
	c := writeOf(fsKey("/repo/a.go"))
	func() {
		defer func() {
			if got := recover(); got != "kaboom" {
				t.Fatalf("recovered %v, want the original panic value", got)
			}
		}()
		_ = r.Hold(context.Background(), c, func(context.Context) error { panic("kaboom") })
	}()
	assertIdle(t, r)
	if err := r.Hold(context.Background(), c, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Hold after panic: %v", err)
	}
}

func TestHoldAlreadyCancelledNeverRuns(t *testing.T) {
	r := NewRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ran := false
	err := r.Hold(ctx, writeOf(fsKey("/a")), func(context.Context) error { ran = true; return nil })
	if !errors.Is(err, context.Canceled) || ran {
		t.Fatalf("Hold = %v ran=%v, want context.Canceled and no run", err, ran)
	}
	assertIdle(t, r)
}

// A waiter cancelled mid-wait must leave the queue: otherwise its ghost
// entry would keep blocking later claims that queue behind it.
func TestHoldCancelledWaiterLeavesNoGhost(t *testing.T) {
	r, contended := contendedSignal()
	k := fsKey("/repo/a.go")
	reader := startHolder(context.Background(), r, readOf(k))
	reader.waitEntered(t)

	ctx, cancel := context.WithCancel(context.Background())
	writerErr := make(chan error, 1)
	go func() {
		writerErr <- r.Hold(ctx, writeOf(k), func(context.Context) error {
			t.Error("cancelled writer ran")
			return nil
		})
	}()
	awaitContended(t, contended, writeOf(k))
	cancel()
	select {
	case err := <-writerErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("writer error = %v, want context.Canceled", err)
		}
	case <-time.After(deadlockBudget):
		t.Fatal("cancelled writer never returned")
	}

	late := startHolder(context.Background(), r, readOf(k))
	late.waitEntered(t) // not stuck behind the departed writer
	late.finish(t)
	reader.finish(t)
	assertIdle(t, r)
}

func TestHoldCancelledDuringCallbackStillReleases(t *testing.T) {
	r := NewRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	err := r.Hold(ctx, writeOf(fsKey("/a")), func(inner context.Context) error {
		cancel()
		return inner.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Hold error = %v, want context.Canceled", err)
	}
	assertIdle(t, r)
}

func TestHoldUncontendedNeverReportsContention(t *testing.T) {
	r := newObservedRegistry(func(c Claim) { t.Errorf("uncontended claim %v reported contention", c) })
	for _, c := range []Claim{readOf(fsKey("/a")), writeOf(fsKey("/a")), writeOf(logicalKey("x"))} {
		if err := r.Hold(context.Background(), c, func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	assertIdle(t, r)
}

// Nesting is reported as ErrNested even when the requested Resource would
// not even resolve: misuse is decided before any resolution I/O.
func TestHoldResourceNestedCheckPrecedesResolution(t *testing.T) {
	r := NewRegistry()
	err := r.HoldResource(context.Background(), Request{Resource: Logical("db"), Mode: Write}, func(ctx context.Context) error {
		return r.HoldResource(ctx, Request{Resource: FS(""), Mode: Read}, func(context.Context) error { return nil })
	})
	if !errors.Is(err, ErrNested) {
		t.Fatalf("HoldResource error = %v, want ErrNested", err)
	}
	assertIdle(t, r)
}

func TestHoldResourceResolvesAgainstWorkspace(t *testing.T) {
	ws := realTempDir(t)
	r := NewRegistry()
	contended := make(chan Claim, 1)
	coarse := startHolder(context.Background(), r, writeOf(fsKey(ws)))
	coarse.waitEntered(t)
	done := make(chan error, 1)
	go func() {
		req := Request{Resource: FS("a.go"), Workspace: ws, Mode: Read, OnContended: func(c Claim) { contended <- c }}
		done <- r.HoldResource(context.Background(), req, func(context.Context) error { return nil })
	}()
	awaitContended(t, contended, readOf(fsKey(filepath.Join(ws, "a.go"))))
	coarse.finish(t)
	if err := <-done; err != nil {
		t.Fatalf("HoldResource: %v", err)
	}
	assertIdle(t, r)
}

func TestHoldResourceRejectsInvalid(t *testing.T) {
	r := NewRegistry()
	err := r.HoldResource(context.Background(), Request{Resource: Logical(""), Mode: Read}, func(context.Context) error {
		t.Error("callback ran for an invalid resource")
		return nil
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("HoldResource error = %v, want ErrInvalid", err)
	}
	assertIdle(t, r)
}

// A request's own OnContended hook reports contention to the caller that
// is actually waiting, and nothing else hears about it.
func TestHoldResourceRequestHookReportsItsOwnContention(t *testing.T) {
	registryWide := make(chan Claim, 1)
	r := newObservedRegistry(func(c Claim) { registryWide <- c })
	name := "db"
	key := logicalKey(name)
	first := startHolder(context.Background(), r, writeOf(key))
	first.waitEntered(t)

	requestHook := make(chan Claim, 1)
	done := make(chan error, 1)
	go func() {
		req := Request{Resource: Logical(name), Mode: Write, OnContended: func(c Claim) { requestHook <- c }}
		done <- r.HoldResource(context.Background(), req, func(context.Context) error { return nil })
	}()
	awaitContended(t, requestHook, writeOf(key))
	first.finish(t)
	if err := <-done; err != nil {
		t.Fatalf("HoldResource: %v", err)
	}
	select {
	case c := <-registryWide:
		t.Fatalf("the first holder's observer heard %v, the second request's contention", c)
	default:
	}
	assertIdle(t, r)
}

func TestCheckFreeReportsHeldClaim(t *testing.T) {
	r := NewRegistry()
	if err := CheckFree(context.Background(), "write fs:/x"); err != nil {
		t.Fatalf("CheckFree on a bare context = %v, want nil", err)
	}
	err := r.Hold(context.Background(), readOf(logicalKey("cache")), func(held context.Context) error {
		return CheckFree(held, "write fs:/x")
	})
	if !errors.Is(err, ErrNested) {
		t.Fatalf("CheckFree while holding = %v, want ErrNested", err)
	}
}
