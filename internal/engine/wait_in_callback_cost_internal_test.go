package engine

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// chainedInCallbackWaits runs n Tasks whose callbacks each Wait, inside
// themselves, on the previous Task while background idle goroutines are
// alive, and returns the wall time of the whole run.
func chainedInCallbackWaits(t *testing.T, n, background int) time.Duration {
	t.Helper()
	stop := make(chan struct{})
	defer close(stop)
	for range background {
		go func() { <-stop }()
	}
	out := quietOutput(Config{MaxConcurrency: 8})
	defer func() { _ = out.Close() }()
	tasks := make([]*TaskHandle, n)
	start := time.Now()
	for i := range n {
		var prev *TaskHandle
		if i > 0 {
			prev = tasks[i-1]
		}
		tasks[i] = out.Task(fmt.Sprint("t", i)).Define(func(context.Context) error {
			if prev != nil {
				if err := prev.Wait(); err != nil {
					return err
				}
			}
			time.Sleep(200 * time.Microsecond)
			return nil
		})
	}
	if err := tasks[n-1].Wait(); err != nil {
		t.Fatal(err)
	}
	_ = out.Finish()
	return time.Since(start)
}

// An in-callback Wait reads its callback from its own stack, so it must not
// pay for a stop-the-world dump of every goroutine in the process: its cost
// is independent of how many goroutines are alive. The bound leaves several
// times the measured cost of 200 chained Waits as headroom.
func TestInCallbackWaitCostIsIndependentOfGoroutineCount(t *testing.T) {
	baseline := chainedInCallbackWaits(t, 200, 0)
	loaded := chainedInCallbackWaits(t, 200, 1000)
	t.Logf("baseline=%v with 1000 idle goroutines=%v", baseline, loaded)
	if loaded > 500*time.Millisecond {
		t.Fatalf("200 in-callback Waits with 1000 idle goroutines took %v (baseline %v): each Wait stops the world to dump every goroutine", loaded, baseline)
	}
}
