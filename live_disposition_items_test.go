package evo_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestLiveGroup_KeptChildrenAggregateInTheLiveFrame pins §25 for the live
// region too: while the category's own work is still running, its already
// kept per-item children are one tally line, never one live row each.
func TestLiveGroup_KeptChildrenAggregateInTheLiveFrame(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{
		Isolated: true, Clock: clock, Terminal: screen, Stdout: io.Discard, Stderr: io.Discard,
		VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever, MaxFrameRate: 1_000_000,
	})
	t.Cleanup(func() { _ = out.Close() })

	branches := out.Group("branches")
	work := branches.Task("branches")
	release := make(chan struct{})
	kept := make(chan struct{})
	work.Define(func(context.Context) error {
		for i := range 5 {
			branches.Task(fmt.Sprintf("feat/%d", i)).Kept(evo.Reason("unpushed"))
		}
		work.Doing("deleting")
		close(kept)
		<-release
		return nil
	})
	<-kept
	clock.Advance(100 * time.Millisecond)
	frame := screen.LatestLiveText()
	close(release)
	_ = work.Wait()
	_ = out.Finish()

	if strings.Contains(frame, "feat/") || !strings.Contains(frame, "! kept 5 (unpushed)") {
		t.Fatalf("live frame must show one kept tally, not per-item rows:\n%s", frame)
	}
}

// TestLiveGroup_CompleteCountNeverDropsAsItemsFold pins the live header's
// "N/M complete": it counts every child, folded or not, so the count a
// reader watches never goes backwards when the second Kept item lands and
// the items fold into a tally.
func TestLiveGroup_CompleteCountNeverDropsAsItemsFold(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{
		Isolated: true, Terminal: screen, Stdout: io.Discard, Stderr: io.Discard,
		VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever, MaxFrameRate: 1_000_000,
	})
	t.Cleanup(func() { _ = out.Close() })

	const running = 3
	packages := out.Group("packages")
	for n := range running {
		packages.Task(fmt.Sprintf("package-%d", n)).Doing("downloading")
	}
	for kept := 1; kept <= 3; kept++ {
		packages.Task(fmt.Sprintf("pinned-%d", kept)).Kept(evo.Reason("pinned"))
		want := fmt.Sprintf("packages  %d/%d complete", kept, running+kept)
		if frame := screen.LatestLiveText(); !strings.Contains(frame, want) {
			t.Fatalf("after %d kept, live header lacks %q:\n%s", kept, want, frame)
		}
	}
}
