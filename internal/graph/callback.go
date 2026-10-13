package graph

import (
	"fmt"
	"runtime"
	"slices"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// terminal reports whether t reached a terminal state.
func terminal(t *Task) bool { return record.IsTerminalTask(t.Rec.State()) }

// builderGateOf is the innermost container builder gate goroutine id is
// running for, or nil.
func (g *Graph) builderGateOf(id GoroutineID) *Task {
	g.lockRead()
	defer g.unlockRead()
	stack := g.exec.consumers[id]
	for _, f := range slices.Backward(stack) {
		if f.task.IsGate() {
			return f.task
		}
	}
	return nil
}

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
	panicked := g.runTrackedBuilder(c.work.Run)
	g.lock()
	defer g.unlock()
	if panicked != nil {
		c.task.workErr = fmt.Errorf("declaring %s: %w", c.task.Name, panicked)
		g.settleLocked(c.task, record.Failed)
		return
	}
	if refusal := c.task.readRefusal; refusal != nil {
		c.task.workErr = fmt.Errorf("declaring %s: %w", c.task.Name, refusal)
		g.settleLocked(c.task, record.Failed)
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

// panicError is a recovered panic value as the error it describes.
func panicError(recovered any) error {
	if err, ok := recovered.(error); ok {
		return fmt.Errorf("panic: %w", err)
	}
	return fmt.Errorf("panic: %v", recovered)
}

// consumerFrame is one Task or builder gate a goroutine is running, with the
// goroutines that goroutine had started when the work began: the work did not
// start those (see goroutineOwnership), whatever it runs beneath.
type consumerFrame struct {
	task      *Task
	predating map[GoroutineID]struct{}
	// known is what the goroutine is known to have started, as of when the
	// work began or when the Task it lent its slot to last ended: a claim
	// that begins while no goroutine has started since needs no new dump to
	// learn what the goroutine had started (see startedBeforeClaim). It is
	// stamped current only when the nested work left no goroutine running
	// (see startedGoroutines.afterNested).
	known startedGoroutines
}

// started reports whether the work in f started goroutine child, which the
// goroutine running f started or whose creator chain passes through one it
// started.
func (f consumerFrame) started(child GoroutineID) bool {
	_, predates := f.predating[child]
	return !predates
}

// knownStartedBy is what the work innermost on goroutine id knows the
// goroutine to have started, and false when the goroutine runs no work.
func (g *Graph) knownStartedBy(id GoroutineID) (startedGoroutines, bool) {
	g.lockRead()
	defer g.unlockRead()
	stack := g.exec.consumers[id]
	if len(stack) == 0 {
		return startedGoroutines{}, false
	}
	return stack[len(stack)-1].known, true
}

// enterConsumer names the Task (or container builder gate) whose callback the
// calling goroutine is running, so a Computed read can ask whether that
// consumer is ordered after the producer. Go has no goroutine-local storage
// and a read takes no context, so the scheduler records it per goroutine.
// predating is the goroutines the calling goroutine had started before the
// work began (see startedBeforeClaim). The returned func ends the entry, and
// tells the work beneath it, if any, what the goroutine had started when the
// entry began.
func (g *Graph) enterConsumer(t *Task, predating startedGoroutines) (leave func()) {
	id := CurrentGoroutine()
	liveAtEntry := runtime.NumGoroutine()
	g.lock()
	if g.exec.consumers == nil {
		g.exec.consumers = make(map[GoroutineID][]consumerFrame)
	}
	g.exec.consumers[id] = append(g.exec.consumers[id], consumerFrame{task: t, predating: predating.ids, known: predating})
	g.unlock()
	return func() {
		ended := predating.afterNested(liveAtEntry)
		g.lock()
		defer g.unlock()
		stack := g.exec.consumers[id]
		if len(stack) <= 1 {
			delete(g.exec.consumers, id)
			return
		}
		stack[len(stack)-2].known = ended
		g.exec.consumers[id] = stack[:len(stack)-1]
	}
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
	id := CurrentGoroutine()
	g.lockRead()
	own, active := g.consumerOnLocked(id), len(g.exec.runningGoroutines) > 0
	g.unlockRead()
	if own != nil || !active {
		return Reader{Task: own}
	}
	_, creator := CurrentGoroutineLineage()
	return Reader{Task: g.startingGoroutine(id, creator).task, Spawned: true}
}

// refuseSpawnedRead keeps err against the Task a refused read from one of its
// goroutines speaks for, so that Task fails with it once its callback returns.
func (g *Graph) refuseSpawnedRead(r Reader, err error) {
	if r.Task == nil || !r.Spawned {
		return
	}
	g.lock()
	defer g.unlock()
	if r.Task.readRefusal == nil {
		r.Task.readRefusal = err
	}
}

// failedByRefusedRead is err, or, when the callback returned none, the read
// refused to a goroutine it started.
func (g *Graph) failedByRefusedRead(t *Task, err error) error {
	g.lockRead()
	defer g.unlockRead()
	if err == nil {
		return t.readRefusal
	}
	return err
}

// consumerOnLocked is the Task or builder gate running on goroutine id, or nil.
func (g *Graph) consumerOnLocked(id GoroutineID) *Task {
	if stack := g.exec.consumers[id]; len(stack) > 0 {
		return stack[len(stack)-1].task
	}
	return nil
}

// frameStartedLocked is the innermost Task or builder gate running on
// goroutine id whose work started goroutine child, or nil: a goroutine a
// plain caller runs a callback on inline owns none of the goroutines it had
// started before, and a callback that lends its slot to nested work keeps the
// goroutines it started before the nested work began (see goroutineOwnership).
func (g *Graph) frameStartedLocked(id, child GoroutineID) *Task {
	if !g.exec.runningGoroutines[id].runsAny() {
		return nil
	}
	for _, f := range slices.Backward(g.exec.consumers[id]) {
		if f.started(child) {
			return f.task
		}
	}
	return nil
}
