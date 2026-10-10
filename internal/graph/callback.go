package graph

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// terminal reports whether t reached a terminal state.
func terminal(t *Task) bool { return record.IsTerminalTask(t.Rec.State()) }

// runTrackedCallback runs fn while the scheduler knows which goroutine is
// running it, so a goroutine fn starts and then parks in a wait can be traced
// back to the callback it may be holding still.
func (g *Graph) runTrackedCallback(fn func() error) error {
	id := CurrentGoroutine()
	g.lock()
	if g.exec.callbackGoroutines == nil {
		g.exec.callbackGoroutines = make(map[GoroutineID]int)
	}
	g.exec.callbackGoroutines[id]++
	g.unlock()
	defer func() {
		g.lock()
		if g.exec.callbackGoroutines[id]--; g.exec.callbackGoroutines[id] <= 0 {
			delete(g.exec.callbackGoroutines, id)
		}
		g.unlock()
	}()
	return runCallback(fn)
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
	panicText := runBuilder(c.work.Run)
	g.lock()
	defer g.unlock()
	if panicText != "" {
		c.task.workErr = fmt.Errorf("declaring %s: panic: %s", c.task.Name, panicText)
		g.settleLocked(c.task, record.Failed)
		return
	}
	g.settleLocked(c.task, record.Done)
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

// CurrentConsumer is the Task or builder gate running on the calling
// goroutine, or nil for a caller outside any callback.
func (g *Graph) CurrentConsumer() *Task {
	id := CurrentGoroutine()
	g.lock()
	defer g.unlock()
	stack := g.exec.consumers[id]
	if len(stack) == 0 {
		return nil
	}
	return stack[len(stack)-1]
}
