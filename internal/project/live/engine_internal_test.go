package live

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/clock"
	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// testScene is a run that never ends and draws no debug pane.
type testScene struct{ ended bool }

func (s *testScene) Ended() bool                 { return s.ended }
func (s *testScene) Pane(int, int) (string, int) { return "", 0 }
func (s *testScene) Resized(int)                 {}

// manualClock is a domain clock a test moves by hand.
type manualClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *manualClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// liveRun is a testRun painted by an Engine on surface, under lock.
type liveRun struct {
	*testRun
	engine *Engine
	lock   *sync.Mutex
}

func newLiveRun(t *testing.T, surface Surface, configure func(*Config)) *liveRun {
	t.Helper()
	r := &liveRun{testRun: newTestRun(), lock: &sync.Mutex{}}
	cfg := Config{
		Surface: surface, Board: r.board, Scene: &testScene{}, Lock: r.lock,
		Clock: &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}, MaxFrameRate: 20,
	}
	if configure != nil {
		configure(&cfg)
	}
	r.engine = NewEngine(cfg)
	t.Cleanup(func() {
		r.lock.Lock()
		r.engine.Stop()
		r.lock.Unlock()
	})
	return r
}

// running declares a Running root Task narrating phase.
func (r *liveRun) running(name, phase string) *record.Task {
	r.lock.Lock()
	defer r.lock.Unlock()
	id := r.declareTask(name, "")
	r.tasks[id].Transition(record.Running)
	r.tasks[id].SetPhase(phase)
	r.engine.Signal(true)
	return r.tasks[id]
}

// TestFrameSnapshottedBeforeAClearIsNeverPainted is the Close, finish and
// durable race in miniature: an unlocked paint still in flight when the
// region is cleared must not land after the clear.
func TestFrameSnapshottedBeforeAClearIsNeverPainted(t *testing.T) {
	s := &paintRecorder{}
	r := newLiveRun(t, s, nil)
	r.running("work", "busy")

	r.lock.Lock()
	f := r.engine.snapshot()
	r.engine.Clear()
	r.lock.Unlock()
	before := len(s.written())

	if r.engine.paint(f, f.render()) {
		t.Fatal("a frame snapshotted before the clear was painted after it")
	}
	if got := len(s.written()); got != before {
		t.Fatalf("writes grew from %d to %d after the clear", before, got)
	}
}

// TestFrameBuiltBeforeFinishIsNotPaintedAfterIt proves the same through
// Finish: a frame the animator built before the final write is dropped.
func TestFrameBuiltBeforeFinishIsNotPaintedAfterIt(t *testing.T) {
	r := newLiveRun(t, &paintRecorder{}, nil)
	r.running("work", "busy")

	r.lock.Lock()
	f := r.engine.snapshot()
	r.engine.Finish("done")
	r.lock.Unlock()

	if r.engine.paint(f, f.render()) {
		t.Fatal("a frame built before Finish was painted after it")
	}
}

// sleepUntilBeforeSlot sleeps until lead before the next spinner glyph slot.
func sleepUntilBeforeSlot(lead time.Duration) {
	wait := untilNextSpinnerSlot(time.Now()) - spinnerSlotSettle - lead
	if wait <= 0 {
		wait += project.SpinnerPeriod
	}
	time.Sleep(wait)
}

// quietCadenceMaxGap is the 100ms rule: no visible frame of a Running row
// stays unchanged this long.
const quietCadenceMaxGap = staleCeiling

// slowSurfaceRun drives a Running Task on a surface whose WriteLive costs
// more than the frame budget, with Progress phased against the animator,
// and returns the WriteLive start times and the animator's skips.
func slowSurfaceRun(t *testing.T) (activated time.Time, starts []time.Time, end time.Time, skipped int64) {
	t.Helper()
	const (
		// Above frameCostBudget, and short enough that a tick after a write
		// can still land inside the skip window.
		writeCost = 14 * time.Millisecond
		// The frame-rate cap sits below the animator's period, so Progress
		// paints inline instead of coalescing. It starts progressLead before
		// a glyph slot, so the animator's tick in that slot finds the
		// surface written recently; progressRest carries the loop past it.
		slowSurfaceFrameRate = 100
		progressLead         = 14 * time.Millisecond
		progressRest         = 60 * time.Millisecond
		runWindow            = 1200 * time.Millisecond
	)
	s := &paintRecorder{delay: writeCost}
	r := newLiveRun(t, s, func(c *Config) {
		c.Clock = clock.System()
		c.MaxFrameRate = slowSurfaceFrameRate
	})
	activated = time.Now()
	task := r.running("slow", "working")
	for i := int64(1); time.Since(activated) < runWindow; i++ {
		sleepUntilBeforeSlot(progressLead)
		r.lock.Lock()
		task.ApplyProgress(i, 1000, record.Determinate)
		r.engine.Signal(false)
		r.lock.Unlock()
		time.Sleep(progressRest)
	}
	end = time.Now()
	return activated, s.writeStarts(), end, r.engine.skippedGlyph.Load()
}

// TestOverloadedSurfaceSkipsGlyphRepaintsWithoutGoingStale proves both
// halves of the backpressure contract: the animator sheds glyph-only
// repaints when frames cost more than the budget, and a Running row's
// visible frame still changes inside the 100ms rule.
func TestOverloadedSurfaceSkipsGlyphRepaintsWithoutGoingStale(t *testing.T) {
	activated, starts, end, skipped := slowSurfaceRun(t)
	if skipped == 0 {
		t.Fatal("the animator never skipped a glyph repaint on an overloaded surface")
	}
	if len(starts) == 0 {
		t.Fatal("no frames were written")
	}
	if lead := starts[0].Sub(activated); lead >= quietCadenceMaxGap {
		t.Fatalf("first frame %s after activation, want <%s", lead, quietCadenceMaxGap)
	}
	for i := 1; i < len(starts); i++ {
		if gap := starts[i].Sub(starts[i-1]); gap >= quietCadenceMaxGap {
			t.Fatalf("frame gap %s at frame %d, want <%s", gap, i, quietCadenceMaxGap)
		}
	}
	if tail := end.Sub(starts[len(starts)-1]); tail >= quietCadenceMaxGap {
		t.Fatalf("last frame %s stale at end of window, want <%s", tail, quietCadenceMaxGap)
	}
	t.Logf("frames=%d skipped=%d", len(starts), skipped)
}

// framesBuiltBySignal is how many frames one signal builds once the clock
// has moved past the frame-rate gap: 1 when the signal paints inline, 0 when
// the animator owns painting.
func framesBuiltBySignal(t *testing.T, tasks int) uint64 {
	t.Helper()
	domain := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	r := newLiveRun(t, &paintRecorder{}, func(c *Config) { c.Clock = domain })
	r.lock.Lock()
	defer r.lock.Unlock()
	r.declareGroup("g_1", "items", "")
	for i := range tasks {
		r.declareTask(fmt.Sprintf("item %d", i), "g_1")
	}
	r.engine.Signal(true)
	domain.Advance(time.Second)
	before := r.engine.seq
	r.engine.Signal(false)
	return r.engine.seq - before
}

func TestSignalPaintsInlineBelowTheForcedRenderBudget(t *testing.T) {
	if got := framesBuiltBySignal(t, 10); got != 1 {
		t.Fatalf("a signal on a small run built %d frames, want 1 inline", got)
	}
}

func TestSignalLeavesPaintingToTheAnimatorPastTheForcedRenderBudget(t *testing.T) {
	if got := framesBuiltBySignal(t, forcedRenderRowsPerInterval+10); got != 0 {
		t.Fatalf("a signal on a large run built %d frames inline, want 0", got)
	}
}

// TestFrameSnapshotIsOwnedByTheFrame is a data-race check: a frame built
// under the run's lock renders identically after the lock is released while
// workers keep mutating the Tasks it was built from.
func TestFrameSnapshotIsOwnedByTheFrame(t *testing.T) {
	const tasks, slowTasks, frames = 16000, 200, 40
	r := newLiveRun(t, &paintRecorder{}, nil)
	r.lock.Lock()
	r.declareGroup("g_1", "items", "")
	var slow []*record.Task
	for i := range tasks {
		id := r.declareTask(fmt.Sprintf("item %d", i), "g_1")
		if i < slowTasks {
			slow = append(slow, r.tasks[id])
		}
	}
	r.lock.Unlock()
	var workers sync.WaitGroup
	for _, task := range slow {
		workers.Go(func() {
			for step := range int64(3) {
				r.lock.Lock()
				task.ApplyProgress(step, 3, record.Determinate)
				r.lock.Unlock()
				time.Sleep(project.SpinnerPeriod / 10)
			}
		})
	}
	defer workers.Wait()
	for range frames {
		r.lock.Lock()
		f := r.engine.frameAt(80, 24, time.Now())
		locked := f.render()
		r.lock.Unlock()
		time.Sleep(time.Millisecond)
		if unlocked := f.render(); unlocked != locked {
			t.Fatalf("frame rendered differently after the lock was released:\nlocked:   %q\nunlocked: %q", locked, unlocked)
		}
	}
}
