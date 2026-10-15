package evo

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

// attributionSettle is how long the test lets a worker go idle or a callback's
// goroutine end before it looks, since the runtime offers nothing to wait on.
const attributionSettle = 100 * time.Millisecond

// A Task whose callback Defines another Task starts a worker from inside that
// callback. A goroutine the nested Task then starts is traced to the first
// Task's worker, which by then runs an unrelated Task. The read must still be
// judged as the callback's that started the goroutine, not as that Task's.
func TestReusedWorkerNeverInheritsAnotherCallbacksLineage(t *testing.T) {
	out := initRun(Config{Output: Plain, MaxConcurrency: 4, Facades: Facades{Stderr: io.Discard}})
	defer func() { _ = out.close() }()
	mk := out.Task("make")
	producer := Compute(mk, func(context.Context) (int, error) { return 7, nil })
	nested := out.Task("n").After(mk)
	unrelated := out.Task("b")
	nestedDefined, unrelatedRunning, learned, finalRead := occurrence(), occurrence(), occurrence(), occurrence()
	var late atomic.Int64

	out.Task("a").After(mk).Define(func(context.Context) error {
		nested.Define(func(context.Context) error {
			<-unrelatedRunning
			go func() {
				defer close(finalRead)
				// A grandchild read teaches the lineage who started whom.
				grandchild := occurrence()
				go func() { defer close(grandchild); _ = producer.Get() }()
				<-grandchild
				close(learned)
				// By now n has returned and the worker that ran it has ended.
				time.Sleep(attributionSettle)
				late.Store(int64(producer.Get()))
			}()
			<-learned
			return nil
		})
		close(nestedDefined)
		return nil
	})
	out.Task("f").Define(func(context.Context) error {
		<-nestedDefined
		// a has returned, so its worker idles and takes the next claim.
		time.Sleep(attributionSettle)
		unrelated.Define(func(context.Context) error {
			close(unrelatedRunning)
			<-finalRead
			return nil
		})
		return nil
	})

	_ = out.finish()

	if late.Load() != 7 || out.err() != nil {
		t.Errorf("late read = %d, Err() = %v; want 7, nil", late.Load(), out.err())
	}
}

// occurrence is a channel closed to announce that something happened.
func occurrence() chan struct{} { return make(chan struct{}) }
