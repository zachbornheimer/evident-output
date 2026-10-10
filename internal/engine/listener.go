package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// declaredState is the state a Task is declared in, and so the state the
// render state first follows it from.
const declaredState = Pending

// The engine's render state follows the record by pulling, never by being
// pushed. A write to the record logs its Task while the record's lock is
// held (record.Run.TakeChanged), so anything that reads the live index first
// pulls that log and finds every write the record has seen, whichever
// goroutine made it and whether or not the record has told its listener yet.
// The listener is only a prompt to pull: the engine hears a change made
// outside Output.mu, takes the mutex itself, and follows the record. No
// handler needs to run inside a writer's lock.

// outputListener is how the engine hears a change made by any writer.
type outputListener struct{ o *Output }

// TaskChanged follows the record when a Task settled. Every other change
// waits in the log for the next read of the index.
func (l outputListener) TaskChanged(_ record.TaskID, from, to record.EntityState) {
	if !settled(from, to) {
		return
	}
	l.o.mu.Lock()
	defer l.o.mu.Unlock()
	l.o.followRecordLocked()
}

// EventAppended has nothing to repaint: the journal is read at Finish.
func (outputListener) EventAppended(record.Event) {}

// settled reports whether a change moved a Task into a terminal state.
func settled(from, to record.EntityState) bool {
	return from != to && core.IsTerminalTask(to)
}

// settleReaction is a Task that settled and the state it settled into.
type settleReaction struct {
	st *taskState
	to EntityState
}

// followRecordLocked brings the render state up to date with the record:
// the live index first, then what each terminal transition owes. Callers
// hold o.mu.
func (o *Output) followRecordLocked() {
	o.pullRecordLocked()
	o.reactToSettlesLocked()
}

// pullRecordLocked brings the live index (census and filing) up to date with
// every write the record has seen, and notes the Tasks that settled so
// reactToSettlesLocked can answer each once. It changes nothing else, so a
// read of the index may pull in the middle of building a frame. Callers hold
// o.mu.
func (o *Output) pullRecordLocked() {
	o.pulled = o.rec.TakeChanged(o.pulled[:0])
	for _, id := range o.pulled {
		st := o.taskStates[string(id)]
		if st == nil {
			continue
		}
		st.markFiling()
		from := st.followed
		st.censusSync()
		if settled(from, st.followed) {
			o.settles = append(o.settles, settleReaction{st: st, to: st.followed})
		}
	}
}

// reactToSettlesLocked answers each Task pullRecordLocked saw settle. Callers
// hold o.mu.
func (o *Output) reactToSettlesLocked() {
	for i := range o.settles {
		o.taskSettledLocked(o.settles[i].st, o.settles[i].to)
		o.settles[i] = settleReaction{}
	}
	o.settles = o.settles[:0]
}

// taskSettledLocked is the render state a terminal transition owes, whichever
// code settled the Task: the live filing, the plain heartbeat, the snapshot
// version and the task.<state> event. The live census moved when the record
// was pulled. Callers hold o.mu.
func (o *Output) taskSettledLocked(st *taskState, to EntityState) {
	if st.isGate() {
		o.bumpLocked()
		return
	}
	st.markFiling()
	o.stopPlainHeartbeatLocked(st)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "task." + string(to), EntityID: st.id})
}
