package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// outputListener is how the engine hears the record change. The record tells
// it after the writer's locks are free, so each handler takes the Output's
// own lock and reacts the way the writer no longer has to.
type outputListener struct{ o *Output }

// TaskChanged reacts to a Task settling: the render state that follows a
// terminal transition is repainted here, whichever code settled the Task.
func (l outputListener) TaskChanged(id record.TaskID, from, to record.EntityState) {
	if from == to || !core.IsTerminalTask(to) {
		return
	}
	o := l.o
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[string(id)]
	if st == nil || st.gateFor != nil {
		return
	}
	st.censusMoved(from)
	st.markFiling()
	o.stopPlainHeartbeatLocked(st)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "task." + string(to), EntityID: st.id})
}

// EventAppended has nothing to repaint: the journal is read at Finish.
func (outputListener) EventAppended(record.Event) {}
