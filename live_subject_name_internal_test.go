package evo

import (
	"strings"
	"testing"
	"time"
)

// TestLive_OneExplicitChildKeepsSubjectName is the red-first proof for the
// canary's three-anonymous-`classify`-rows defect: three sibling subjects
// (worktrees/branches/remote-tracking) that each declare one explicitly
// named child called `classify` collapsed, in the live region, onto the
// child row alone — so every frame for the whole classify phase read
// `classify` three times and named no subject. The live path must apply the
// transcript's stricter rule: a differently named child never stands in for
// its group's name. The group header keeps the subject, and the single
// Running child's phase/progress rides on that same header row so the frame
// stays one line per subject.
func TestLive_OneExplicitChildKeepsSubjectName(t *testing.T) {
	drv := &fakeHeartbeatSurface{}
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("zq", withTerminal(drv), visibilityDelay(0), withClock(clock), withNoColor())
	t.Cleanup(func() { _ = out.Close() })

	classify := out.Group("worktrees").Task("classify")
	classify.Doing("../.worktrees/agent-a254279")
	classify.Progress(24, 111)

	out.mu.Lock()
	out.renderLiveLocked(true)
	frame := drv.latest()
	out.mu.Unlock()

	line := strings.SplitN(strings.TrimRight(frame, "\n"), "\n", 2)[0]
	for _, want := range []string{"worktrees", "classify", "24/111", "agent-a254279"} {
		if !strings.Contains(line, want) {
			t.Fatalf("want %q on the single live subject line, got:\n%s", want, frame)
		}
	}
	if n := strings.Count(strings.TrimRight(frame, "\n"), "\n"); n != 0 {
		t.Fatalf("one subject with one Running child must render one line, got %d extra:\n%s", n, frame)
	}
}

// TestLive_PendingRowHasNoTimer is the red-first proof for the canary's
// `○ classify  waiting — 12s` rows: a Pending row does not accumulate work
// time (dialect spec, Heartbeat rule: "Pending/NotStarted rows do not
// accumulate 'work time'"). It still says it is waiting, and the region
// still animates, but the elapsed suffix belongs to Running work only.
func TestLive_PendingRowHasNoTimer(t *testing.T) {
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
		t.Fatalf("a stale Pending row still says it is waiting:\n%s", frame)
	}
	if strings.Contains(frame, "waiting — ") {
		t.Fatalf("a Pending row must not accumulate work time:\n%s", frame)
	}
	if !animating {
		t.Fatal("expected needsSpinnerAnimLocked true while an unresolved Pending row is rendered")
	}
}
