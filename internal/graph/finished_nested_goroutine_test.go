package graph

import (
	"runtime"
	"sync"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// idleBeforeFirst is how many unrelated goroutines end while the first nested
// Task runs in the churn variant: more than the one it leaves running.
const idleBeforeFirst = 50

// readerOfLeftGoroutine runs a lender that Waits on two nested Tasks at
// ceiling 1. The first starts a goroutine and leaves it running, after
// running firstWork. The second releases that goroutine to read who it speaks
// for, and the Reader it got is returned with the second Task.
func readerOfLeftGoroutine(t *testing.T, firstWork func()) (Reader, *Task) {
	t.Helper()
	g := New(record.NewRun(), WithMaxConcurrency(1))
	outer, first, second := declare(g, "outer"), declare(g, "first"), declare(g, "second")
	readNow, got := make(chan struct{}), make(chan Reader)
	var reader Reader
	submitWork(g, first, settlingWork(g, first, func() error {
		firstWork()
		go func() {
			<-readNow
			got <- g.CurrentReader()
		}()
		return nil
	}))
	submitWork(g, second, settlingWork(g, second, func() error {
		close(readNow)
		reader = <-got
		return nil
	}))
	submitWork(g, outer, settlingWork(g, outer, func() error {
		if err := g.Wait(first); err != nil {
			return err
		}
		return g.Wait(second)
	}))
	g.Kick()
	g.Drain()
	return reader, second
}

// A goroutine the first nested Task started and left running is not the
// second nested Task's: the second Task did not start it, so the lender's
// known set cannot be reused as if the first Task had left nothing behind.
func TestGoroutineAFinishedNestedTaskLeftRunningIsNotTheNextNestedTasks(t *testing.T) {
	reader, second := readerOfLeftGoroutine(t, func() {})
	if reader.Task == second {
		t.Fatalf("a goroutine the first Task started and left running read as %q, which did not start it", second.Name)
	}
}

// Other goroutines ending while the first nested Task runs do not hide the one
// it leaves running: the count of running goroutines falls, and a fall is no
// proof that the Task left none.
func TestGoroutineAFinishedNestedTaskLeftRunningIsFoundWhileOthersEnd(t *testing.T) {
	stop := make(chan struct{})
	var started, exited sync.WaitGroup
	for range idleBeforeFirst {
		started.Add(1)
		exited.Go(func() {
			started.Done()
			<-stop
		})
	}
	started.Wait()
	reader, second := readerOfLeftGoroutine(t, func() {
		live := runtime.NumGoroutine()
		close(stop)
		exited.Wait()
		for runtime.NumGoroutine() >= live {
			runtime.Gosched()
		}
	})
	if reader.Task == second {
		t.Fatalf("a goroutine the first Task left running read as %q, which did not start it, once %d others ended", second.Name, idleBeforeFirst)
	}
}
