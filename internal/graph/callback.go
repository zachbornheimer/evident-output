package graph

import (
	"fmt"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// terminal reports whether t reached a terminal state.
func terminal(t *Task) bool { return record.IsTerminalTask(t.Rec.State()) }

// runCallback is the single frame every task callback runs beneath, so a
// goroutine parked in a wait can count how many callbacks it is holding
// still. That count is the one thing separating "a callback is stuck
// waiting" from "a plain caller is waiting while callbacks run".
func runCallback(fn func() error) error {
	callbackFrames.Note()
	return fn()
}

// runGate runs a claimed container builder and settles its gate. The builder
// runs under builderFrames, not runCallback, so the declarations it makes are
// not Task-callback declarations.
func (g *Graph) runGate(c *claim) {
	panicked := runBuilder(c.work.Run)
	g.lock()
	defer g.unlock()
	if panicked != nil || c.task.readRefusal.held() != nil {
		g.failGateLocked(c.task, "")
		return
	}
	g.settleLocked(c.task, record.Succeeded)
}

// runBuilder runs work and returns what it panicked with, nil when it did
// not. A panic value that is an error stays matchable with errors.Is, like a
// callback's sentinel is: a refused Computed read unwinds the builder with one.
func runBuilder(work func() error) (panicked error) {
	defer func() {
		if r := recover(); r != nil {
			panicked = panicError(r)
		}
	}()
	builderFrames.Note()
	if work != nil {
		_ = work()
	}
	return nil
}

// panicError is a recovered panic value as the error it describes. A refused Computed read stays itself:
// it is the graph's refusal, not a panic the caller wrote.
func panicError(recovered any) error {
	if err, ok := recovered.(error); ok {
		if IsRefusedRead(err) {
			return err
		}
		return fmt.Errorf("panic: %w", err)
	}
	return fmt.Errorf("panic: %v", recovered)
}

// Reader is who a Computed read speaks for: the Task or builder gate whose
// callback makes it, nil for a caller outside every callback.
type Reader struct {
	Task *Task
	// Spawned is whether the read comes from a goroutine the Task's callback
	// started. That goroutine is the callback's hands, but cannot unwind it.
	Spawned bool
}

// Unwinds reports whether a refused read can unwind its caller to fail the
// reader's Task, as a read on the callback's own goroutine can.
func (r Reader) Unwinds() bool { return r.Task != nil && !r.Spawned }

// CurrentReader is the Reader of the calling goroutine: the Task or builder
// gate running on it or, failing that, on the goroutine that started it. A
// run with no callback in flight reads no lineage.
func (g *Graph) CurrentReader() Reader {
	own := g.frames.on(CurrentGoroutine())
	if own != nil || !g.frames.anyRunning() {
		return Reader{Task: own}
	}
	_, creator := CurrentGoroutineLineage()
	return Reader{Task: g.startingGoroutine(creator).task, Spawned: true}
}

// refuseSpawnedRead keeps err against the Task a refused read from one of its
// goroutines speaks for, so that Task fails with it once its callback returns.
func (g *Graph) refuseSpawnedRead(r Reader, err error) {
	if r.Task == nil || !r.Spawned {
		return
	}
	r.Task.readRefusal.keep(err)
}

// failedByRefusedRead is err, or, when the callback returned none, the read
// refused to a goroutine it started.
func (g *Graph) failedByRefusedRead(t *Task, err error) error {
	if err == nil {
		return t.readRefusal.held()
	}
	return err
}

// refusal is the first error kept against a Task, the Task's own: kept and
// read without the graph's lock, since every callback reads it as it
// returns.
type refusal struct{ first atomic.Pointer[error] }

// keep holds err unless an earlier one is held.
func (r *refusal) keep(err error) { r.first.CompareAndSwap(nil, &err) }

// held is the kept error, nil when none was.
func (r *refusal) held() error {
	if err := r.first.Load(); err != nil {
		return *err
	}
	return nil
}
