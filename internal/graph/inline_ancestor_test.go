package graph

import (
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// inlineCallbackRun is a plain caller (the test goroutine, like evo.Run's body
// goroutine) that Waits on first, which the scheduler has not started, so it
// runs first's callback inline on its own goroutine. That callback only
// sleeps: it never waits on anything. A goroutine descended from the plain
// caller through depth goroutines, all started before the callback began,
// asks from inside it.
type inlineCallbackRun struct {
	g              *Graph
	first, second  *Task
	running        chan struct{}
	callbackLasted time.Duration
}

func newInlineCallbackRun() *inlineCallbackRun {
	r := &inlineCallbackRun{running: make(chan struct{}), callbackLasted: 300 * time.Millisecond}
	r.g = New(record.NewRun(), WithMaxConcurrency(2))
	r.first, r.second = declare(r.g, "first"), declare(r.g, "second")
	r.g.AddAfter(r.second, AfterTask(r.first))
	submitWork(r.g, r.first, settlingWork(r.g, r.first, func() error {
		close(r.running)
		time.Sleep(r.callbackLasted)
		return nil
	}))
	submitWork(r.g, r.second, settlingWork(r.g, r.second, func() error { return nil }))
	return r
}

// askFromDescendant starts a chain of depth goroutines from the plain caller
// before it runs first inline; the last one runs ask once first is running.
// It returns what ask returned.
func (r *inlineCallbackRun) askFromDescendant(t *testing.T, depth int, ask func() any) any {
	t.Helper()
	got := make(chan any, 1)
	var spawn func(level int)
	spawn = func(level int) {
		if level < depth {
			done := make(chan struct{})
			go func() { spawn(level + 1); close(done) }()
			<-done
			return
		}
		<-r.running
		got <- ask()
	}
	go spawn(1)
	if err := r.g.Wait(r.first); err != nil {
		t.Fatalf("Wait(first) = %v", err)
	}
	select {
	case v := <-got:
		r.g.Drain()
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("the descendant's call hung")
		return nil
	}
}

// A goroutine descended from a plain caller before the plain caller ran a
// callback inline is not that callback's: nothing is deadlocked while the
// callback runs, so the descendant's Wait on work behind it must succeed.
func TestWaitFromDescendantOfInlineRunningCallerIsNotDeadlocked(t *testing.T) {
	for _, depth := range []int{1, 2, 3} {
		r := newInlineCallbackRun()
		got := r.askFromDescendant(t, depth, func() any { return r.g.Wait(r.second) })
		if err, _ := got.(error); err != nil {
			t.Fatalf("depth %d: Wait(second) from a descendant = %v, want nil", depth, err)
		}
	}
}

// The same descendant reading a Computed is a plain caller's read, not the
// inline callback's: it is not refused for an order the callback has.
func TestReadFromDescendantOfInlineRunningCallerIsNotTheCallbacks(t *testing.T) {
	for _, depth := range []int{1, 2, 3} {
		r := newInlineCallbackRun()
		got := r.askFromDescendant(t, depth, func() any { return r.g.CurrentReader() })
		if reader := got.(Reader); reader.Task != nil {
			t.Fatalf("depth %d: a descendant read as %q, want no Task", depth, reader.Task.Name)
		}
	}
}
