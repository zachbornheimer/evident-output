package record

import "sync"

// Listener hears that the record changed, so a projection can react without
// polling it. A listener is never called while the record's own lock is
// held, and never while a Hold is open: a writer that holds its own lock
// across a group of writes wraps them in Hold and Release, so the listener
// hears them only after that lock is gone and may take it. Calls arrive in
// the order the writes happened, one at a time, on the goroutine whose
// Release or write left no Hold open. A listener must be quick and must not
// block.
//
// A notification is a prompt, not the only way to learn of a write: the
// record's change log (see TakeChanged) holds every written Task from the
// moment of the write, so a reader that must be exact pulls it.
type Listener interface {
	// TaskChanged reports that a write changed the Task named id. A write
	// that moves the Task between states reports the state it left and the
	// state it entered; any other write reports the Task's current state as
	// both.
	TaskChanged(id TaskID, from, to EntityState)
	// TaskSettled reports that the Task named id was settled: the one write
	// that ends it, whatever state it ended in. It follows the TaskChanged
	// of the same write. A projection reacts to a settle here rather than
	// guessing from the states, so it can never disagree with the writer
	// about which states end a Task.
	TaskSettled(id TaskID, from, to EntityState)
	// EventAppended reports an event the journal just stamped and kept.
	EventAppended(e Event)
}

// listenerBox lets an interface value sit in an atomic pointer.
type listenerBox struct{ Listener }

// notification is one change waiting to be told to the listener: a Task
// change (event nil) or an appended event.
type notification struct {
	task     TaskID
	from, to EntityState
	settled  bool
	event    *Event
}

// notifier holds the notifications a Hold keeps back and delivers them in
// order once the last Hold is released.
type notifier struct {
	mu         sync.Mutex
	holds      int
	delivering bool
	pending    []notification
	// spare is the delivered batch's storage, reused by the next batch so a
	// write costs no allocation once the queue has grown to its working size.
	spare []notification
}

// SetListener makes l hear every later change; nil stops listening.
func (r *Run) SetListener(l Listener) {
	if l == nil {
		r.listener.Store(nil)
		return
	}
	r.listener.Store(&listenerBox{l})
}

// Hold defers every notification until the matching Release. Holds nest and
// count across goroutines: the listener hears nothing while any is open.
func (r *Run) Hold() {
	r.notes.mu.Lock()
	defer r.notes.mu.Unlock()
	r.notes.holds++
}

// Release ends one Hold. The Release that ends the last one tells the
// listener everything held back, in order, on its own goroutine. A Release
// with no Hold open does nothing.
func (r *Run) Release() {
	if !r.notes.drop() {
		return
	}
	r.deliverPending()
}

// drop ends one Hold and reports whether that left none open.
func (n *notifier) drop() (idle bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.holds == 0 {
		return false
	}
	n.holds--
	return n.holds == 0
}

func (r *Run) notifyTaskChanged(id TaskID, from, to EntityState) {
	r.post(notification{task: id, from: from, to: to})
}

func (r *Run) notifyEventAppended(e Event) {
	r.post(notification{event: &e})
}

// post queues n and, when no Hold is open, tells the listener at once.
func (r *Run) post(n notification) {
	if r.listener.Load() == nil {
		return
	}
	r.notes.mu.Lock()
	r.notes.pending = append(r.notes.pending, n)
	idle := r.notes.holds == 0
	r.notes.mu.Unlock()
	if idle {
		r.deliverPending()
	}
}

// deliverPending tells the run's listener what is queued, one goroutine at a
// time so the order never interleaves. It stops when a Hold opens, leaving
// the rest to that Hold's Release. No lock is held while the listener runs.
func (r *Run) deliverPending() {
	n := &r.notes
	n.mu.Lock()
	if n.delivering {
		n.mu.Unlock()
		return
	}
	n.delivering = true
	for len(n.pending) > 0 && n.holds == 0 {
		batch := n.pending
		n.pending = n.spare[:0]
		n.mu.Unlock()
		r.tell(batch)
		n.mu.Lock()
		clear(batch)
		n.spare = batch[:0]
	}
	n.delivering = false
	n.mu.Unlock()
}

// tell delivers batch to the run's listener. If the listener panics,
// delivery is handed back so a later write can deliver again.
func (r *Run) tell(batch []notification) {
	to := r.listener.Load()
	if to == nil {
		return
	}
	returned := false
	defer func() {
		if !returned {
			r.notes.mu.Lock()
			r.notes.delivering = false
			r.notes.mu.Unlock()
		}
	}()
	for _, n := range batch {
		if n.event != nil {
			to.EventAppended(*n.event)
			continue
		}
		if n.settled {
			to.TaskSettled(n.task, n.from, n.to)
			continue
		}
		to.TaskChanged(n.task, n.from, n.to)
	}
	returned = true
}

// Mutex is a mutex whose critical section is also a Hold on a Run: what the
// section writes to the record reaches the run's listener only after the
// mutex is free again, so the listener may take it. The zero Mutex is an
// ordinary unlocked mutex until Bind names its Run.
type Mutex struct {
	mu  sync.Mutex
	run *Run
}

// Bind makes r the Run this Mutex's sections hold. Call it before first use.
func (m *Mutex) Bind(r *Run) { m.run = r }

// Lock takes the mutex, then holds the Run's notifications.
func (m *Mutex) Lock() {
	m.mu.Lock()
	if m.run != nil {
		m.run.Hold()
	}
}

// Unlock frees the mutex, then releases the hold, which tells the run's
// listener whatever the section wrote.
func (m *Mutex) Unlock() {
	if m.run == nil {
		m.mu.Unlock()
		return
	}
	defer m.run.Release()
	m.mu.Unlock()
}

// changed tells the listener this Task changed without changing state.
// Writers defer it before taking the run lock, so it runs after the lock is
// released.
func (t *Task) changed() {
	if t.run.listener.Load() == nil {
		return
	}
	state := t.State()
	t.run.notifyTaskChanged(t.id, state, state)
}

// changedState tells the listener this Task moved from one state to another.
func (t *Task) changedState(from, to EntityState) { t.run.notifyTaskChanged(t.id, from, to) }

// settledState tells the listener this Task was settled, from one state to another.
func (t *Task) settledState(from, to EntityState) {
	t.run.post(notification{task: t.id, from: from, to: to, settled: true})
}
