package graph

import (
	"errors"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// startCallbackThatBlocksOnAGoroutineWaiting declares outer and inner at a
// ceiling of one slot. outer's callback starts a goroutine that Waits on
// inner, which needs the only slot, and blocks on that goroutine: nothing can
// move. The error the goroutine's Wait returned reaches the returned channel.
func startCallbackThatBlocksOnAGoroutineWaiting(g *Graph) (outer *Task, childErr <-chan error) {
	outer, inner := declare(g, "outer"), declare(g, "inner")
	got := make(chan error, 1)
	submitWork(g, outer, settlingWork(g, outer, func() error {
		wait := make(chan error)
		go func() { wait <- g.Wait(inner) }()
		err := <-wait
		got <- err
		return err
	}))
	submitWork(g, inner, settlingWork(g, inner, func() error { return nil }))
	return outer, got
}

func requireReleasedAsDeadlock(t *testing.T, childErr <-chan error) {
	t.Helper()
	select {
	case err := <-childErr:
		if !errors.Is(err, ErrWaitDeadlock) {
			t.Fatalf("the callback's goroutine Wait(inner) = %v, want ErrWaitDeadlock", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("hang: the callback's own goroutine's Wait was never released")
	}
}

// A callback a plain caller runs inline (it Waits on work the scheduler has
// not started) is still the callback that started its goroutines: when they
// wait on work behind the slot it holds, the wait is released with
// ErrWaitDeadlock, as it is for a callback on a pooled worker.
func TestWaitFromInlineCallbacksOwnGoroutineIsReleasedAsDeadlock(t *testing.T) {
	g := New(record.NewRun(), WithMaxConcurrency(1))
	outer, childErr := startCallbackThatBlocksOnAGoroutineWaiting(g)
	go func() { _ = g.Wait(outer) }() // a plain caller; runs outer inline
	requireReleasedAsDeadlock(t, childErr)
}

// The same shape on a pooled worker: the baseline an inline callback matches.
func TestWaitFromPooledCallbacksOwnGoroutineIsReleasedAsDeadlock(t *testing.T) {
	g := New(record.NewRun(), WithMaxConcurrency(1))
	_, childErr := startCallbackThatBlocksOnAGoroutineWaiting(g)
	g.Kick()
	requireReleasedAsDeadlock(t, childErr)
}

// startBuilderWhoseGoroutineWaits declares a settled Task and a container
// whose builder starts a goroutine that Waits on it, and blocks on that
// goroutine. The error the goroutine's Wait returned reaches the returned
// channel.
func startBuilderWhoseGoroutineWaits(t *testing.T, g *Graph) (gate *Task, waitErr <-chan error) {
	t.Helper()
	settled := declare(g, "settled")
	submitWork(g, settled, settlingWork(g, settled, func() error { return nil }))
	if err := g.Wait(settled); err != nil {
		t.Fatal(err)
	}
	got := make(chan error, 1)
	group := g.AddContainer(nil, "builder", false)
	gate = g.AddGate(group, func() error {
		wait := make(chan error)
		go func() { wait <- g.Wait(settled) }()
		got <- <-wait
		return nil
	})
	g.Submit(gate, Work{})
	return gate, got
}

func requireRefusedAsInsideBuilder(t *testing.T, waitErr <-chan error) {
	t.Helper()
	select {
	case err := <-waitErr:
		if !errors.Is(err, ErrWaitInBuilder) {
			t.Fatalf("the builder's goroutine Wait(settled) = %v, want ErrWaitInBuilder", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("hang: the builder's goroutine's Wait never returned")
	}
}

// A container builder a plain caller runs inline is still the builder that
// started its goroutines: a Wait from one is refused as inside the builder,
// as it is for a builder on a pooled worker.
func TestWaitFromInlineBuildersGoroutineIsRefusedAsInsideTheBuilder(t *testing.T) {
	g := New(record.NewRun(), WithMaxConcurrency(1))
	gate, waitErr := startBuilderWhoseGoroutineWaits(t, g)
	go func() { _ = g.Wait(gate) }() // a plain caller; runs the builder inline
	requireRefusedAsInsideBuilder(t, waitErr)
}

// The same shape on a pooled worker: the baseline an inline builder matches.
func TestWaitFromPooledBuildersGoroutineIsRefusedAsInsideTheBuilder(t *testing.T) {
	g := New(record.NewRun(), WithMaxConcurrency(1))
	_, waitErr := startBuilderWhoseGoroutineWaits(t, g)
	g.Kick()
	requireRefusedAsInsideBuilder(t, waitErr)
}

// A goroutine a callback started that runs another Task inline (it Waits on
// work the scheduler has not started) is that Task's runner: a goroutine the
// inline Task's callback starts answers to it, not to the callback above.
func TestReadFromGoroutineAnInlineRunTaskStartedIsThatTasks(t *testing.T) {
	g := New(record.NewRun(), WithMaxConcurrency(4))
	outer, inner := declare(g, "outer"), declare(g, "inner")
	innerSubmitted, outerReturned := make(chan struct{}), make(chan struct{})
	var reader Reader
	submitWork(g, outer, settlingWork(g, outer, func() error {
		defer close(outerReturned)
		<-innerSubmitted
		done := make(chan error)
		go func() { done <- g.Wait(inner) }() // inner is not started: this goroutine runs it
		return <-done
	}))
	g.Kick()
	submitWork(g, inner, settlingWork(g, inner, func() error {
		got := make(chan Reader)
		go func() { got <- g.CurrentReader() }()
		reader = <-got
		return nil
	}))
	close(innerSubmitted)
	<-outerReturned // no Kick or Drain before then: only the waiting goroutine may run inner
	g.Drain()
	if reader.Task != inner || !reader.Spawned {
		name := "none"
		if reader.Task != nil {
			name = reader.Task.Name
		}
		t.Fatalf("a goroutine the inline Task started read as %q (spawned %v), want %q", name, reader.Spawned, inner.Name)
	}
}
