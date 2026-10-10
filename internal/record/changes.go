package record

import (
	"sync"
	"sync/atomic"
)

// changeLog is the Tasks written since a projection last asked. A writer
// adds its Task while it still holds the run lock, so a reader that sees a
// write in the record also finds its Task here; a notification, delivered
// after the lock is gone, cannot promise that. A projection that must be
// exact when it reads (a live index over the Tasks) pulls this log first and
// treats the notification only as a prompt to do so.
type changeLog struct {
	mu    sync.Mutex
	tasks []*Task
	// revision counts every write, logged once or not.
	revision atomic.Uint64
}

// add logs t once until the next take.
func (l *changeLog) add(t *Task) {
	l.revision.Add(1)
	l.mu.Lock()
	defer l.mu.Unlock()
	if t.logged {
		return
	}
	t.logged = true
	l.tasks = append(l.tasks, t)
}

// take appends the logged Tasks to into and forgets them.
func (l *changeLog) take(into []TaskID) []TaskID {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, t := range l.tasks {
		into = append(into, t.id)
		t.logged = false
		l.tasks[i] = nil
	}
	l.tasks = l.tasks[:0]
	return into
}

// TakeChanged appends to into the Task of every write made since the last
// call, each once and in the order it first changed, and forgets them. A
// Task written again after the call is reported again. Read each Task's
// state afterwards: a write that lands while the caller walks the result is
// reported by the next call, never lost.
func (r *Run) TakeChanged(into []TaskID) []TaskID { return r.changes.take(into) }

// Revision counts the writes to Tasks made so far. Two reads that return the
// same number saw no Task written in between, so a reader that walks the
// record twice can tell when a concurrent write made its two walks disagree.
func (r *Run) Revision() uint64 { return r.changes.revision.Load() }

// lockForWrite takes the run lock for a write to t and logs t as changed
// before any reader can see the write.
func (t *Task) lockForWrite() {
	t.run.mu.Lock()
	t.run.changes.add(t)
}
