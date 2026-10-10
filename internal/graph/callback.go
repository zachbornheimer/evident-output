package graph

import (
	"fmt"
	"slices"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// terminal reports whether t reached a terminal state.
func terminal(t *Task) bool { return record.IsTerminalTask(t.Rec.State()) }

// runTrackedCallback runs fn while the scheduler knows which goroutine is
// running it, so a goroutine fn starts and then parks in a wait can be traced
// back to the callback it may be holding still.
func (g *Graph) runTrackedCallback(fn func() error) error {
	defer g.trackRunning(goroutineLoad{callbacks: 1})()
	return runCallback(fn)
}

// goroutineLoad is what one goroutine is running right now: task callbacks
// and container builders.
type goroutineLoad struct{ callbacks, builders int }

func (l goroutineLoad) total() int { return l.callbacks + l.builders }

// trackRunning adds added to the calling goroutine's load, and returns the
// func that takes it back out.
func (g *Graph) trackRunning(added goroutineLoad) (done func()) {
	id := CurrentGoroutine()
	g.lock()
	if g.exec.runningGoroutines == nil {
		g.exec.runningGoroutines = make(map[GoroutineID]goroutineLoad)
	}
	load := g.exec.runningGoroutines[id]
	load.callbacks += added.callbacks
	load.builders += added.builders
	g.exec.runningGoroutines[id] = load
	g.exec.buildersRunning += added.builders
	g.unlock()
	return func() {
		g.lock()
		defer g.unlock()
		g.exec.buildersRunning -= added.builders
		load := g.exec.runningGoroutines[id]
		load.callbacks -= added.callbacks
		load.builders -= added.builders
		if load.total() <= 0 {
			delete(g.exec.runningGoroutines, id)
			return
		}
		g.exec.runningGoroutines[id] = load
	}
}

// anyBuilderRunning reports whether any goroutine is running a container
// builder right now.
func (g *Graph) anyBuilderRunning() bool {
	g.lockRead()
	defer g.unlockRead()
	return g.exec.buildersRunning > 0
}

// builderGateOf is the innermost container builder gate goroutine id is
// running for, or nil.
func (g *Graph) builderGateOf(id GoroutineID) *Task {
	g.lockRead()
	defer g.unlockRead()
	stack := g.exec.consumers[id]
	for _, s := range slices.Backward(stack) {
		if s.IsGate() {
			return s
		}
	}
	return nil
}

// isRunningBuilder reports whether goroutine id is running a container
// builder.
func (g *Graph) isRunningBuilder(id GoroutineID) bool {
	g.lockRead()
	defer g.unlockRead()
	return g.exec.runningGoroutines[id].builders > 0
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
	panicText := g.runTrackedBuilder(c.work.Run)
	g.lock()
	defer g.unlock()
	if panicText != "" {
		c.task.workErr = fmt.Errorf("declaring %s: panic: %s", c.task.Name, panicText)
		g.settleLocked(c.task, record.Failed)
		return
	}
	if refusal := c.task.readRefusal; refusal != nil {
		c.task.workErr = fmt.Errorf("declaring %s: %w", c.task.Name, refusal)
		g.settleLocked(c.task, record.Failed)
		return
	}
	g.settleLocked(c.task, record.Done)
}

// runTrackedBuilder is runBuilder while the scheduler knows which goroutine is
// running the builder, as it does for a callback, so a goroutine the builder
// starts is traced back to it.
func (g *Graph) runTrackedBuilder(work func() error) string {
	defer g.trackRunning(goroutineLoad{builders: 1})()
	return runBuilder(work)
}

func runBuilder(work func() error) (panicText string) {
	defer func() {
		if r := recover(); r != nil {
			panicText = fmt.Sprint(r)
		}
	}()
	builderFrames.Note()
	if work != nil {
		_ = work()
	}
	return ""
}

// enterConsumer names the Task (or container builder gate) whose callback the
// calling goroutine is running, so a Computed read can ask whether that
// consumer is ordered after the producer. Go has no goroutine-local storage
// and a read takes no context, so the scheduler records it per goroutine. The
// returned func ends the entry.
func (g *Graph) enterConsumer(t *Task) (leave func()) {
	id := CurrentGoroutine()
	g.lock()
	if g.exec.consumers == nil {
		g.exec.consumers = make(map[GoroutineID][]*Task)
	}
	g.exec.consumers[id] = append(g.exec.consumers[id], t)
	g.unlock()
	return func() {
		g.lock()
		defer g.unlock()
		stack := g.exec.consumers[id]
		if len(stack) <= 1 {
			delete(g.exec.consumers, id)
			return
		}
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
	own, active := g.consumerThroughLocked(id, 0), len(g.exec.consumers) > 0
	g.unlockRead()
	if own != nil || !active {
		return Reader{Task: own}
	}
	_, creator := CurrentGoroutineLineage()
	g.lockRead()
	defer g.unlockRead()
	return Reader{Task: g.consumerThroughLocked(creator, 0), Spawned: true}
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

// consumerThroughLocked is the Task or builder gate running on goroutine
// self, or, when self runs none, on the goroutine that started it (creator,
// zero for none).
func (g *Graph) consumerThroughLocked(self, creator GoroutineID) *Task {
	for _, id := range [...]GoroutineID{self, creator} {
		if stack := g.exec.consumers[id]; len(stack) > 0 {
			return stack[len(stack)-1]
		}
	}
	return nil
}
