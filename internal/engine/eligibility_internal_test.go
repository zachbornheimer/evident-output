package engine

import (
	"context"
	"io"
	"strconv"
	"testing"
)

// A full scheduler must not rescan the run on every kick (a 100+ worktree
// zq clean-repo at MaxConcurrency 1 kicks once per settle). Queued work is
// stamped eligible when it is submitted, and a kick at capacity leaves the
// ready queue untouched.
func TestScheduler_KickAtCapacityLeavesReadyQueueUntouched(t *testing.T) {
	out := Init(Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, MaxConcurrency: 1})
	t.Cleanup(func() { _ = out.Close() })
	release := make(chan struct{})
	started := make(chan struct{})
	out.Task("blocker").Define(func(context.Context) error {
		close(started)
		<-release
		return nil
	})
	<-started
	const queued = 200
	tasks := out.Group("worktrees")
	for i := range queued {
		tasks.Task("wt" + strconv.Itoa(i)).Define(func(context.Context) error { return nil })
	}

	out.mu.Lock()
	depth := out.schedReady.items.Len()
	unstamped := 0
	for _, st := range out.tasks {
		if st.submitted && st.timing.EligibleAt.IsZero() {
			unstamped++
		}
	}
	out.mu.Unlock()
	if depth != queued || unstamped != 0 {
		t.Fatalf("ready depth = %d (want %d), unstamped eligible = %d (want 0)", depth, queued, unstamped)
	}
	if st, _ := out.takeEligible(); st != nil {
		t.Fatalf("takeEligible at capacity claimed %q", st.name)
	}
	out.mu.Lock()
	after := out.schedReady.items.Len()
	out.mu.Unlock()
	if after != queued {
		t.Fatalf("ready depth after a kick at capacity = %d, want %d", after, queued)
	}
	close(release)
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}
