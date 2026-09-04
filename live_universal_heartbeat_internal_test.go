package evo

import (
	"strings"
	"sync"
	"testing"
	"time"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// fakeHeartbeatSurface is a minimal LiveSurface test double that records the
// last text painted via WriteLive, mirroring fakeSinkTerminal's shape but
// exposing the frame for assertions instead of discarding it.
type fakeHeartbeatSurface struct {
	mu   sync.Mutex
	last string
}

func (f *fakeHeartbeatSurface) ID() string          { return "fake-heartbeat" }
func (f *fakeHeartbeatSurface) Columns() int        { return 80 }
func (f *fakeHeartbeatSurface) Rows() int           { return 24 }
func (f *fakeHeartbeatSurface) IsInteractive() bool { return true }
func (f *fakeHeartbeatSurface) ClearLive()          {}
func (f *fakeHeartbeatSurface) WriteDurable(string) {}
func (f *fakeHeartbeatSurface) WriteFinal(string)   {}
func (f *fakeHeartbeatSurface) WriteLive(text string) {
	f.mu.Lock()
	f.last = text
	f.mu.Unlock()
}
func (f *fakeHeartbeatSurface) latest() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last
}

// manualClock is a TimeSource that only moves when Advance is called —
// distinct from live_spinner_test.go's advancingClock (which steps on every
// Now()), because these tests need a stable "now" to force a deterministic
// render before and after a single jump past elapsedAfter.
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

// TestLiveHeartbeat_PendingRowAnimatesPastElapsedThreshold is the red-first
// proof for evo-rec.md Problem 9's static-frame defect: a live region
// holding only a Pending row (nothing Running) must still grow an elapsed
// suffix past elapsedAfter AND keep the spinner animator alive — before the
// fix, heartbeatSuffix required a non-zero ActivityAt (which a never-started
// Pending task never has) and needsSpinnerAnimLocked only counted Running,
// so the frame froze forever. P5 anchors every row to LiveFirstSeenAt
// instead, so a Pending row ages honestly from the moment it is first
// painted.
func TestLiveHeartbeat_PendingRowAnimatesPastElapsedThreshold(t *testing.T) {
	drv := &fakeHeartbeatSurface{}
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", Terminal(drv), VisibilityDelay(0), Clock(clock), NoColor())
	t.Cleanup(func() { _ = out.Close() })

	out.Task("goimports") // declared, never touched: stays Pending

	clock.Advance(15 * time.Second)

	out.mu.Lock()
	out.renderLiveLocked(true)
	frame := drv.latest()
	animating := out.needsSpinnerAnimLocked()
	out.mu.Unlock()

	if !strings.Contains(frame, "waiting — 15s") {
		t.Fatalf("expected a dim waiting heartbeat on the stale Pending row:\n%s", frame)
	}
	if !animating {
		t.Fatal("expected needsSpinnerAnimLocked true while an unresolved Pending row is rendered")
	}
}

// TestLiveHeartbeat_RunningNoPhaseZeroTotalAnimates is the red-first proof
// for the Determinate(0,0)/no-phase gap: writeLiveTaskLine's switch has no
// case for a Running task whose progress is Determinate with Total==0 and no
// Phase, so it falls to the bare glyph+name default branch — no working
// text, no heartbeat, ever.
func TestLiveHeartbeat_RunningNoPhaseZeroTotalAnimates(t *testing.T) {
	drv := &fakeHeartbeatSurface{}
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", Terminal(drv), VisibilityDelay(0), Clock(clock), NoColor())
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("resolve")
	task.Progress(0, 0) // Determinate, Total==0, no Phase — promotes to Running

	clock.Advance(15 * time.Second)

	out.mu.Lock()
	out.renderLiveLocked(true)
	frame := drv.latest()
	out.mu.Unlock()

	if !strings.Contains(frame, "working… — 15s") {
		t.Fatalf("expected working… heartbeat for Determinate(0,0)/no-phase Running row:\n%s", frame)
	}
}

// TestLiveHeartbeat_CollectionHeaderAnimatesOnUnresolvedPendingChild is the
// red-first proof that a collection's live header stays animated while any
// child is unresolved, in both the mixed (some Done, one still Pending) and
// all-Pending-at-start shapes — anyChildPendingActive currently requires
// t.Phase != "" on top of Pending, so a child that never called Phase (the
// normal case) never animates the header, and it freezes on the derivedState
// "-" (Incomplete) glyph instead.
func TestLiveHeartbeat_CollectionHeaderAnimatesOnUnresolvedPendingChild(t *testing.T) {
	spinnerFrame := func(now time.Time) string { return txt.SpinnerGlyph(now, GlyphsUnicode) }

	t.Run("mixed done and pending", func(t *testing.T) {
		drv := &fakeHeartbeatSurface{}
		clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
		out := newOutput("fix", Terminal(drv), VisibilityDelay(0), Clock(clock), NoColor(), Glyphs(GlyphsUnicode))
		t.Cleanup(func() { _ = out.Close() })

		grp := out.DisplayGroup("fix")
		grp.Task("a").Done()
		grp.Task("b").Done()
		grp.Task("c").Done()
		grp.Task("goimports") // stays Pending

		out.mu.Lock()
		out.renderLiveLocked(true)
		frame := drv.latest()
		out.mu.Unlock()

		header := strings.SplitN(frame, "\n", 2)[0]
		if !strings.HasPrefix(header, spinnerFrame(clock.Now())) {
			t.Fatalf("expected animated spinner header, not a static glyph:\n%q", header)
		}
	})

	t.Run("all pending at start", func(t *testing.T) {
		drv := &fakeHeartbeatSurface{}
		clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
		out := newOutput("fix", Terminal(drv), VisibilityDelay(0), Clock(clock), NoColor(), Glyphs(GlyphsUnicode))
		t.Cleanup(func() { _ = out.Close() })

		grp := out.DisplayGroup("fix")
		grp.Task("a")
		grp.Task("b")

		out.mu.Lock()
		out.renderLiveLocked(true)
		frame := drv.latest()
		out.mu.Unlock()

		header := strings.SplitN(frame, "\n", 2)[0]
		if !strings.HasPrefix(header, spinnerFrame(clock.Now())) {
			t.Fatalf("expected an all-Pending collection to animate its header at start:\n%q", header)
		}
	})
}
