package record

// Listener hears that the record changed, so a projection can react without
// polling it. Both methods run after the record released its own lock, on
// the goroutine that wrote, so a listener may call back into the record and
// take its own locks. A listener must be quick and must not block.
type Listener interface {
	// TaskChanged reports that a write changed the Task named id.
	TaskChanged(id TaskID)
	// EventAppended reports an event the journal just stamped and kept.
	EventAppended(e Event)
}

// listenerBox lets an interface value sit in an atomic pointer.
type listenerBox struct{ Listener }

// SetListener makes l hear every later change; nil stops listening.
func (r *Run) SetListener(l Listener) {
	if l == nil {
		r.listener.Store(nil)
		return
	}
	r.listener.Store(&listenerBox{l})
}

func (r *Run) notifyTaskChanged(id TaskID) {
	if box := r.listener.Load(); box != nil {
		box.TaskChanged(id)
	}
}

func (r *Run) notifyEventAppended(e Event) {
	if box := r.listener.Load(); box != nil {
		box.EventAppended(e)
	}
}

// changed tells the listener this Task changed. Writers defer it before
// taking the run lock, so it runs after the lock is released.
func (t *Task) changed() { t.run.notifyTaskChanged(t.id) }
