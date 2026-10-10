package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/graph"
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

// A Task a builder declared runs its callback on a pooled worker the
// builder's Define kicked. That callback is not the builder: a goroutine the
// callback started may Wait on unrelated, settled work while the builder is
// still declaring, and must not be refused as a Wait inside the builder.
func TestWaitFromAGoroutineABuilderChildCallbackStartedIsNotInsideTheBuilder(t *testing.T) {
	err := waitFromBuilderChild(t, func(pre *TaskHandle, got chan<- error) {
		done := make(chan struct{})
		go func() { got <- pre.Wait(); close(done) }()
		<-done
	})
	if errors.Is(err, graph.ErrWaitInBuilder) {
		t.Fatalf("Wait from a goroutine the child's callback started = %v: refused as inside the builder", err)
	}
	if err != nil {
		t.Fatalf("Wait = %v, want nil", err)
	}
}

// The same child's callback Waiting on its own goroutine is no more inside
// the builder: its stack names the callback it runs.
func TestWaitFromABuilderChildCallbackIsNotInsideTheBuilder(t *testing.T) {
	err := waitFromBuilderChild(t, func(pre *TaskHandle, got chan<- error) { got <- pre.Wait() })
	if err != nil {
		t.Fatalf("Wait = %v, want nil", err)
	}
}
