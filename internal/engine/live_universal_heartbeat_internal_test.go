package engine

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
// holding only a Pending row (nothing Running) must still say it is waiting
// past elapsedAfter AND keep the spinner animator alive — before the fix,
// heartbeatSuffix required a non-zero ActivityAt (which a never-started
// Pending task never has) and needsSpinnerAnimLocked only counted Running,
// so the frame froze forever. P5 anchors every row to LiveFirstSeenAt
// instead, so a Pending row ages honestly from the moment it is first
// painted.
//
// Revised: the row's waiting text no longer carries the elapsed suffix this
// test originally pinned (`waiting — 15s`). The dialect's Heartbeat rule
// says Pending/NotStarted rows do not accumulate "work time", and the
// suffix made three queued siblings read as three stalled jobs; the timer
// now belongs to the Running work and the parent header alone
// (TestLive_PendingRowHasNoTimer). What this test still owns is the
// original defect: the row is not silent, and the animator stays alive.
func TestLiveHeartbeat_PendingRowAnimatesPastElapsedThreshold(t *testing.T) {
	drv := &fakeHeartbeatSurface{}
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", withTerminal(drv), visibilityDelay(0), withClock(clock), withNoColor())
	t.Cleanup(func() { _ = out.Close() })

	out.Sequence("steps").Task("goimports") // sequential child stays Pending until its turn

	clock.Advance(15 * time.Second)

	out.mu.Lock()
	out.renderLiveLocked(true)
	frame := drv.latest()
	animating := out.needsSpinnerAnimLocked()
	out.mu.Unlock()

	if !strings.Contains(frame, "waiting") {
		t.Fatalf("expected a dim waiting marker on the stale Pending row:\n%s", frame)
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
	out := newOutput("job", withTerminal(drv), visibilityDelay(0), withClock(clock), withNoColor())
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
// "-" (Incomplete) glyph instead. Sequence keeps that header; a
// non-sequential Group of independently named children flattens to sibling
// rows (TestLiveHeartbeat_GroupSiblingsRenderAsAlignedParentRows).
func TestLiveHeartbeat_CollectionHeaderAnimatesOnUnresolvedPendingChild(t *testing.T) {
	spinnerFrame := func(now time.Time) string { return txt.SpinnerGlyph(now, GlyphsUnicode) }

	t.Run("mixed done and pending", func(t *testing.T) {
		drv := &fakeHeartbeatSurface{}
		clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
		out := newOutput("fix", withTerminal(drv), visibilityDelay(0), withClock(clock), withNoColor(), Glyphs(GlyphsUnicode))
		t.Cleanup(func() { _ = out.Close() })

		seq := out.Sequence("fix")
		seq.Task("a").Done()
		seq.Task("b").Done()
		seq.Task("c").Done()
		seq.Task("goimports") // stays Pending

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
		out := newOutput("fix", withTerminal(drv), visibilityDelay(0), withClock(clock), withNoColor(), Glyphs(GlyphsUnicode))
		t.Cleanup(func() { _ = out.Close() })

		seq := out.Sequence("fix")
		seq.Task("a")
		seq.Task("b")

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

// liveRunningHeartbeat is spec §23.1's interactive Running-frame contract:
// a visibly different live frame within 100ms of entering Running, and at
// least every 100ms thereafter, without the app emitting fake Progress.
const liveRunningHeartbeat = 100 * time.Millisecond

// TestLiveHeartbeat_GroupSiblingsRenderAsAlignedParentRows is the red-first
// proof for spec §23's live parallel prune shape: a non-sequential Group of
// independently named children is those children as the story — aligned
// parent rows with determinate progress, a stable timer, and one indented
// current-activity child — never a "N/M complete" group header that jams
// Phase onto the parent bar.
func TestLiveHeartbeat_GroupSiblingsRenderAsAlignedParentRows(t *testing.T) {
	drv := &fakeHeartbeatSurface{}
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("prune", withTerminal(drv), visibilityDelay(0), withClock(clock), withNoColor(), Glyphs(GlyphsUnicode))
	t.Cleanup(func() { _ = out.Close() })

	grp := out.Group("prune")
	branches := grp.Task("branches")
	worktrees := grp.Task("worktrees")
	remotes := grp.Task("remote-tracking")

	branches.Step(120, 459, "feat/style-contract")
	worktrees.Step(70, 294, "eapp-system-style-contract-heading")
	remotes.Step(1, 4, "origin/old-style")

	clock.Advance(5 * time.Second)
	out.mu.Lock()
	out.renderLiveLocked(true)
	frame := drv.latest()
	out.mu.Unlock()

	if strings.Contains(frame, "complete") {
		t.Fatalf("independently named Group siblings must not spend a live header on N/M complete:\n%s", frame)
	}
	for _, name := range []string{"branches", "worktrees", "remote-tracking"} {
		if !strings.Contains(frame, name) {
			t.Fatalf("want parent name %q in the live frame:\n%s", name, frame)
		}
	}
	for _, count := range []string{"120/459", "70/294", "1/4"} {
		if !strings.Contains(frame, count) {
			t.Fatalf("want count %q in the live frame:\n%s", count, frame)
		}
	}

	var sawIndentedActivity bool
	for _, line := range strings.Split(frame, "\n") {
		if strings.HasPrefix(line, "   ") && strings.Contains(line, "feat/style-contract") {
			sawIndentedActivity = true
			if strings.Contains(line, " — ") {
				t.Fatalf("timer must stay on the parent line, not the activity child:\n%s", frame)
			}
		}
		if strings.Contains(line, "120/459") && !strings.Contains(line, " — ") {
			t.Fatalf("parent row must keep the elapsed timer:\n%s", frame)
		}
	}
	if !sawIndentedActivity {
		t.Fatalf("want an indented current-item line under at least one parent:\n%s", frame)
	}
}

// TestLiveHeartbeat_GroupKeepsHeaderWithoutTwoDeterminateChildren is the
// red-first proof that H.20's mixed Group still owns a header: one
// determinate Bytes child plus one indeterminate Phase child is not the
// spec §23 prune flatten. The group name and N/M complete stay.
func TestLiveHeartbeat_GroupKeepsHeaderWithoutTwoDeterminateChildren(t *testing.T) {
	drv := &fakeHeartbeatSurface{}
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", withTerminal(drv), visibilityDelay(0), withClock(clock), withNoColor(), Glyphs(GlyphsUnicode))
	t.Cleanup(func() { _ = out.Close() })

	grp := out.Group("dependencies")
	react := grp.Task("react")
	esbuild := grp.Task("esbuild")
	sharp := grp.Task("sharp")
	sharp.Doing("verifying")
	esbuild.Bytes(12_400_000, 18_000_000)
	react.Bytes(8_100_000, 8_100_000)
	react.Done()

	out.mu.Lock()
	out.renderLiveLocked(true)
	frame := drv.latest()
	out.mu.Unlock()

	header := strings.SplitN(frame, "\n", 2)[0]
	if !strings.Contains(header, "dependencies") || !strings.Contains(header, "1/3 complete") {
		t.Fatalf("want the group header with N/M complete, got:\n%s", frame)
	}
}

// TestLiveHeartbeat_KeepHeaderWhenIndeterminateSiblingRemains is the
// red-first proof that two Bytes-running children do not flatten the Group
// while a third is still phase-only (H.20's live window: react+esbuild
// have Bytes, sharp is Doing("verifying")). Mixed determinate + phase-only
// keeps the N/M complete header.
func TestLiveHeartbeat_KeepHeaderWhenIndeterminateSiblingRemains(t *testing.T) {
	drv := &fakeHeartbeatSurface{}
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", withTerminal(drv), visibilityDelay(0), withClock(clock), withNoColor(), Glyphs(GlyphsUnicode))
	t.Cleanup(func() { _ = out.Close() })

	grp := out.Group("dependencies")
	react := grp.Task("react")
	esbuild := grp.Task("esbuild")
	sharp := grp.Task("sharp")
	react.Bytes(8_100_000, 8_100_000)
	esbuild.Bytes(12_400_000, 18_000_000)
	sharp.Doing("verifying")

	out.mu.Lock()
	out.renderLiveLocked(true)
	frame := drv.latest()
	out.mu.Unlock()

	if !strings.Contains(frame, "complete") {
		t.Fatalf("mixed determinate + phase-only Group must keep the N/M complete header:\n%s", frame)
	}
}

// TestLiveHeartbeat_SettledGroupKeepsHeader is the red-first proof that a
// fully resolved Group still wraps its children — V8's "✓ launch agent"
// shape. Flatten is only for in-flight determinate siblings.
func TestLiveHeartbeat_SettledGroupKeepsHeader(t *testing.T) {
	drv := &fakeHeartbeatSurface{}
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", withTerminal(drv), visibilityDelay(0), withClock(clock), withNoColor())
	t.Cleanup(func() { _ = out.Close() })

	agent := out.Group("launch agent")
	agent.Task("write plist").Done()
	agent.Task("register").Done()
	agent.Task("start").Done()

	out.mu.Lock()
	out.renderLiveLocked(true)
	frame := drv.latest()
	out.mu.Unlock()

	header := strings.SplitN(frame, "\n", 2)[0]
	if !strings.Contains(header, "launch agent") {
		t.Fatalf("settled Group must keep its header:\n%s", frame)
	}
}

// TestLiveHeartbeat_RunningFrameChangesWithin100ms is the red-first proof
// that Evo owns the live TTY heartbeat: a Running task with no Progress
// still paints within 100ms of becoming Running, and the next domain-clock
// 100ms produces a different frame (spinner glyph or equivalent). The
// animator must tick off the injected Clock — tests never sleep.
func TestLiveHeartbeat_RunningFrameChangesWithin100ms(t *testing.T) {
	drv := &fakeHeartbeatSurface{}
	clock := newHeartbeatFakeClock()
	out := newOutput("job", withTerminal(drv), withClock(clock), withNoColor(), Glyphs(GlyphsUnicode))
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("resolve")
	task.Doing("working") // Running; no Progress — Evo must keep the frame alive.

	first := drv.latest()
	if first == "" {
		t.Fatal("expected first WriteLive within 100ms of entering Running")
	}

	clock.Advance(liveRunningHeartbeat)
	second := drv.latest()
	if second == first {
		t.Fatalf("live frame must change within 100ms of Running (spinner glyph or equivalent):\n%s", second)
	}
}
