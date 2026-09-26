package evo_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestLiveGroup_SkippedChildrenAggregateInTheLiveFrame pins §25 for the
// live region too, and contract §18: while the category's own work is
// still Running, its already-skipped per-item children show neither one
// live row each nor a "- skipped N" tally — the tally's count could still
// be understating what has actually skipped so far, so it waits for the
// category to settle (the same moment its own row stops spinning) rather
// than print a number that might still grow. Once the Task resolves, the
// fold appears as one tally line, never one row per item.
func TestLiveGroup_SkippedChildrenAggregateInTheLiveFrame(t *testing.T) {
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
	skipped := make(chan struct{})
	var releaseOnce sync.Once
	releaseWorker := func() { releaseOnce.Do(func() { close(release) }) }
	// Registered after out.Close's own t.Cleanup (line above), so it runs
	// first (t.Cleanup is LIFO): a Fatalf below leaves Define's goroutine
	// still blocked on <-release, and out.Close's cleanup would otherwise
	// wait on it forever. Releasing first unblocks Define so out.Close can
	// complete; sync.Once makes the explicit close(...) later in this test
	// a no-op instead of a double-close panic.
	t.Cleanup(releaseWorker)
	work.Define(func(context.Context) error {
		for i := range 5 {
			branches.Task(fmt.Sprintf("feat/%d", i)).Skipped(evo.Reason("unpushed"))
		}
		work.Doing("deleting")
		close(skipped)
		<-release
		return nil
	})
	<-skipped
	clock.Advance(100 * time.Millisecond)
	running := screen.LatestLiveText()
	if strings.Contains(running, "feat/") || strings.Contains(running, "- skipped") {
		t.Fatalf("live frame must show no per-item rows and no tally while the category is still running:\n%s", running)
	}

	releaseWorker()
	_ = work.Wait()
	clock.Advance(100 * time.Millisecond)
	settled := screen.LatestLiveText()
	_ = out.Finish()

	if strings.Contains(settled, "feat/") || !strings.Contains(settled, "- skipped 5 (unpushed)") {
		t.Fatalf("live frame must show one skipped tally once the category settles, not per-item rows:\n%s", settled)
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
		packages.Task(fmt.Sprintf("pinned-%d", kept)).Skipped(evo.Reason("pinned"))
		want := fmt.Sprintf("packages  %d/%d complete", kept, running+kept)
		if frame := screen.LatestLiveText(); !strings.Contains(frame, want) {
			t.Fatalf("after %d kept, live header lacks %q:\n%s", kept, want, frame)
		}
	}
}
