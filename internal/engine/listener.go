package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// The engine hears the record change in two places, one handler each. The
// Output's mutex tells heldListener inside every critical section, just
// before it frees the mutex, so the render state follows a write before
// anyone else can read it. outputListener is the run's own listener: it
// hears only what a write outside any critical section left behind, and takes
// the mutex itself.

// outputListener is how the engine hears a change made outside Output.mu.
type outputListener struct{ o *Output }

// TaskChanged repaints a Task that settled without the Output's lock held.
func (l outputListener) TaskChanged(id record.TaskID, from, to record.EntityState) {
	if !settled(from, to) {
		return
	}
	l.o.mu.Lock()
	defer l.o.mu.Unlock()
	l.o.taskSettledLocked(id, from, to)
}

// EventAppended has nothing to repaint: the journal is read at Finish.
func (outputListener) EventAppended(record.Event) {}

// heldListener is how the engine hears a change made inside Output.mu. It
// runs with the lock held, so it must not take it.
type heldListener struct{ o *Output }

// TaskChanged repaints a Task that settled inside the critical section.
func (l heldListener) TaskChanged(id record.TaskID, from, to record.EntityState) {
	if settled(from, to) {
		l.o.taskSettledLocked(id, from, to)
	}
}

// EventAppended has nothing to repaint: the journal is read at Finish.
func (heldListener) EventAppended(record.Event) {}

// settled reports whether a change moved a Task into a terminal state.
func settled(from, to record.EntityState) bool {
	return from != to && core.IsTerminalTask(to)
}

// taskSettledLocked is the render state a terminal transition owes, whichever
// code settled the Task: the live census and filing, the plain heartbeat,
// the snapshot version and the task.<state> event. Callers hold o.mu.
func (o *Output) taskSettledLocked(id record.TaskID, from, to record.EntityState) {
	st := o.taskStates[string(id)]
	if st == nil {
		return
	}
	if st.isGate() {
		o.bumpLocked()
		return
	}
	st.censusMoved(from)
	st.markFiling()
	o.stopPlainHeartbeatLocked(st)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "task." + string(to), EntityID: st.id})
}
