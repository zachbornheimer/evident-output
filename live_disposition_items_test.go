package evo

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/testkit"
)

// TestLiveGroup_KeptChildrenAggregateInTheLiveFrame pins §25 for the live
// region too: while the category's own work is still running, its already
// kept per-item children are one tally line, never one live row each.
func TestLiveGroup_KeptChildrenAggregateInTheLiveFrame(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := initRun(Config{
		isolated: true, clock: clock, terminal: screen, stdout: io.Discard, stderr: io.Discard,
		visibilityDelay: DelayForTest(0), color: colorNever, maxFrameRate: 1_000_000,
	})
	t.Cleanup(func() { _ = out.close() })

	branches := out.Group("branches")
	work := branches.Task("branches")
	release := make(chan struct{})
	kept := make(chan struct{})
	for i := range 5 {
		branches.Task(fmt.Sprintf("feat/%d", i)).skipped(reason("unpushed"))
	}
	work.Define(func(context.Context) error {
		work.Doing("deleting")
		close(kept)
		<-release
		return nil
	})
	<-kept
	clock.Advance(100 * time.Millisecond)
	frame := screen.LatestLiveText()
	close(release)
	_ = work.wait()
	_ = out.finish()

	if strings.Contains(frame, "feat/") || !strings.Contains(frame, "- skipped 5 (unpushed)") {
		t.Fatalf("live frame must show one skipped tally, not per-item rows:\n%s", frame)
	}
}

// TestLiveGroup_CompleteCountNeverDropsAsItemsFold pins the live header's
// "N/M complete": it counts every child, folded or not, so the count a
// reader watches never goes backwards when the second Kept item lands and
// the items fold into a tally.
func TestLiveGroup_CompleteCountNeverDropsAsItemsFold(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := initRun(Config{
		isolated: true, terminal: screen, stdout: io.Discard, stderr: io.Discard,
		visibilityDelay: DelayForTest(0), color: colorNever, maxFrameRate: 1_000_000,
	})
	t.Cleanup(func() { _ = out.close() })

	const running = 3
	packages := out.Group("packages")
	for n := range running {
		packages.Task(fmt.Sprintf("package-%d", n)).Doing("downloading")
	}
	for kept := 1; kept <= 3; kept++ {
		packages.Task(fmt.Sprintf("pinned-%d", kept)).skipped(reason("pinned"))
		want := fmt.Sprintf("packages  %d/%d complete", kept, running+kept)
		if frame := screen.LatestLiveText(); !strings.Contains(frame, want) {
			t.Fatalf("after %d kept, live header lacks %q:\n%s", kept, want, frame)
		}
	}
}
