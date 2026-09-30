package engine

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// paintRecorder is a LiveSurface that records what reached it. A nonzero
// delay makes each WriteLive cost that much wall time.
type paintRecorder struct {
	mu     sync.Mutex
	writes []string
	clears int
	delay  time.Duration
	starts []time.Time
}

func (s *paintRecorder) ID() string          { return "paint-recorder" }
func (s *paintRecorder) Columns() int        { return 80 }
func (s *paintRecorder) Rows() int           { return 24 }
func (s *paintRecorder) IsInteractive() bool { return true }
func (s *paintRecorder) WriteDurable(string) {}
func (s *paintRecorder) WriteFinal(string)   {}
func (s *paintRecorder) WriteLive(text string) {
	s.mu.Lock()
	s.starts = append(s.starts, time.Now())
	s.writes = append(s.writes, text)
	s.mu.Unlock()
	time.Sleep(s.delay)
}
func (s *paintRecorder) ClearLive() {
	s.mu.Lock()
	s.clears++
	s.mu.Unlock()
}

func (s *paintRecorder) written() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.writes...)
}

func (s *paintRecorder) writeStarts() []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Time(nil), s.starts...)
}

func (s *paintRecorder) clearCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clears
}

func frameAt(s LiveSurface, seq uint64) liveFrame { return liveFrame{surface: s, seq: seq} }

func TestPaintDropsAFrameOlderThanOnePainted(t *testing.T) {
	s, l := &paintRecorder{}, &liveEngine{}
	if !l.paint(frameAt(s, 2), "new") {
		t.Fatal("frame 2 was not painted")
	}
	if l.paint(frameAt(s, 1), "old") {
		t.Fatal("frame 1 painted after frame 2")
	}
	if got := s.written(); len(got) != 1 || got[0] != "new" {
		t.Fatalf("writes = %q, want only the newer frame", got)
	}
}

func TestPaintDropsFramesAtOrBelowTheClearFloor(t *testing.T) {
	s, l := &paintRecorder{}, &liveEngine{seq: 5}
	l.paint(frameAt(s, 5), "before")
	l.clear(s)
	for _, seq := range []uint64{4, 5} {
		if l.paint(frameAt(s, seq), "stale") {
			t.Fatalf("frame %d painted at or below the floor", seq)
		}
	}
	if !l.paint(frameAt(s, 6), "after") {
		t.Fatal("frame 6, past the floor, was dropped")
	}
}

func TestPaintOfIdenticalTextStillAdvancesThePaintedSequence(t *testing.T) {
	s, l := &paintRecorder{}, &liveEngine{}
	l.paint(frameAt(s, 1), "same")
	if l.paint(frameAt(s, 3), "same") {
		t.Fatal("identical text was written again")
	}
	if l.paint(frameAt(s, 2), "older") {
		t.Fatal("frame 2 painted after frame 3 was seen")
	}
	if got := len(s.written()); got != 1 {
		t.Fatalf("%d writes, want 1", got)
	}
}

func TestClearReportsWhetherAnythingWasOnScreen(t *testing.T) {
	s, l := &paintRecorder{}, &liveEngine{}
	if l.clear(s) {
		t.Fatal("clear of an empty screen reported a region")
	}
	l.paint(frameAt(s, 1), "frame")
	if !l.clear(s) {
		t.Fatal("clear did not report the painted region")
	}
	if l.clear(s) {
		t.Fatal("second clear reported a region")
	}
	if got := s.clearCount(); got != 1 {
		t.Fatalf("ClearLive called %d times, want 1", got)
	}
}

// TestFrameSnapshottedBeforeAClearIsNeverPainted is the Close/finish/durable
// race in miniature: an unlocked paint still in flight when the region is
// cleared must not land after the clear.
func TestFrameSnapshottedBeforeAClearIsNeverPainted(t *testing.T) {
	s := &paintRecorder{}
	out := newOutput("job", withTerminal(s), visibilityDelay(0), withNoColor())
	t.Cleanup(func() { _ = out.Close() })
	out.Task("work").Doing("busy")

	out.mu.Lock()
	f := out.liveFrameLocked(s)
	out.live.clear(s)
	out.mu.Unlock()
	before := len(s.written())

	if out.live.paint(f, f.render()) {
		t.Fatal("a frame snapshotted before the clear was painted after it")
	}
	if got := len(s.written()); got != before {
		t.Fatalf("writes grew from %d to %d after the clear", before, got)
	}
}

// TestFrameBuiltBeforeCloseIsNotPaintedAfterIt proves the same through the
// real Close path: a frame the animator built before finish is dropped.
func TestFrameBuiltBeforeCloseIsNotPaintedAfterIt(t *testing.T) {
	s := &paintRecorder{}
	out := newOutput("job", withTerminal(s), visibilityDelay(0), withNoColor())
	out.Task("work").Doing("busy")

	out.mu.Lock()
	f := out.liveFrameLocked(s)
	engine := out.live
	out.mu.Unlock()
	_ = out.Close()

	if engine.paint(f, f.render()) {
		t.Fatal("a frame built before Close was painted after it")
	}
}

// sleepUntilBeforeSlot sleeps until lead before the next spinner glyph slot.
func sleepUntilBeforeSlot(lead time.Duration) {
	wait := untilNextSpinnerSlot(time.Now()) - spinnerSlotSettle - lead
	if wait <= 0 {
		wait += txt.SpinnerPeriod
	}
	time.Sleep(wait)
}

// slowSurfaceRun drives a Running task on a surface whose WriteLive costs
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
	out := newOutput("job", withTerminal(s), visibilityDelay(0), withNoColor(), maxFrameRate(slowSurfaceFrameRate))
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("slow")
	activated = time.Now()
	task.Doing("working")
	for i := 1; time.Since(activated) < runWindow; i++ {
		sleepUntilBeforeSlot(progressLead)
		task.Progress(i, 1000)
		time.Sleep(progressRest)
	}
	end = time.Now()
	out.mu.Lock()
	skipped = out.live.skippedGlyph.Load()
	out.mu.Unlock()
	return activated, s.writeStarts(), end, skipped
}

// TestLive_OverloadedSurfaceSkipsGlyphRepaintsWithoutGoingStale proves both
// halves of the backpressure contract: the animator sheds glyph-only
// repaints when frames cost more than the budget, and a Running row's
// visible frame still changes inside the 100ms rule.
func TestLive_OverloadedSurfaceSkipsGlyphRepaintsWithoutGoingStale(t *testing.T) {
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
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", withTerminal(&paintRecorder{}), visibilityDelay(0), withClock(clock), withNoColor())
	t.Cleanup(func() { _ = out.Close() })
	g := out.Group("items")
	for i := range tasks {
		g.Task(fmt.Sprintf("item %d", i))
	}
	clock.Advance(time.Second)
	out.mu.Lock()
	defer out.mu.Unlock()
	before := out.live.seq
	out.signalLiveLocked(false)
	return out.live.seq - before
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

// TestLiveFrameSnapshotIsOwnedByTheFrame is a data-race check: a frame
// built under o.mu renders identically after o.mu is released while
// workers keep mutating the Tasks it was built from.
func TestLiveFrameSnapshotIsOwnedByTheFrame(t *testing.T) {
	const tasks, slowTasks, frames = 16000, 200, 40
	out := newOutput("job", withTerminal(&paintRecorder{}), visibilityDelay(0), withNoColor(), maxConcurrency(4))
	t.Cleanup(func() { _ = out.Close() })
	g := out.Group("items")
	for i := range tasks {
		h := g.Task(fmt.Sprintf("item %d", i))
		h.Define(func(context.Context) error {
			if i >= slowTasks {
				return nil
			}
			for step := range 3 {
				h.Progress(step, 3)
				time.Sleep(txt.SpinnerPeriod / 10)
			}
			return nil
		})
	}
	for range frames {
		out.mu.Lock()
		f := out.liveFrameAtLocked(80, 24, time.Now())
		locked := f.render()
		out.mu.Unlock()
		time.Sleep(time.Millisecond)
		if unlocked := f.render(); unlocked != locked {
			t.Fatalf("frame rendered differently after o.mu was released:\nlocked:   %q\nunlocked: %q", locked, unlocked)
		}
	}
}
