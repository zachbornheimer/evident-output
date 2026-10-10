package engine

import (
	"context"
	"testing"
	"time"
)

// waitFromBuilderChild declares a Group whose builder keeps declaring (slow
// enumeration) after its child Task starts, runs wait inside the child's
// callback, and returns what the child's Wait on already-settled work got.
func waitFromBuilderChild(t *testing.T, wait func(pre *TaskHandle, got chan<- error)) error {
	t.Helper()
	out := quietOutput(Config{MaxConcurrency: 4})
	t.Cleanup(func() { _ = out.Close() })
	pre := out.Task("pre").Define(noop)
	if err := pre.Wait(); err != nil {
		t.Fatal(err)
	}
	got := make(chan error, 1)
	out.Group("builder").Define(func(g *GroupHandle) {
		g.Task("child").Define(func(context.Context) error {
			wait(pre, got)
			return nil
		})
		time.Sleep(time.Second)
	})
	if _, finished := within(10*time.Second, out.Finish); !finished {
		t.Fatal("Finish hung")
	}
	select {
	case err := <-got:
		return err
	default:
		t.Fatal("child never waited")
		return nil
	}
}

// A builder child's callback runs on a pooled worker the builder's Define
// kicked. Its stack names the callback it runs, so a Wait on its own
// goroutine is the callback's, not the builder's.
func TestWaitFromABuilderChildCallbackIsNotInsideTheBuilder(t *testing.T) {
	err := waitFromBuilderChild(t, func(pre *TaskHandle, got chan<- error) { got <- pre.Wait() })
	if err != nil {
		t.Fatalf("Wait = %v, want nil", err)
	}
}
