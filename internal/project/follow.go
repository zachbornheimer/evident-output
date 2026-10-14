package project

import (
	"sync"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// Follower brings a run's projections up to date with its record by
// pulling, never by being pushed. A write to the record logs its Task while
// the record's lock is held (record.Run.TakeChanged), so a reader that pulls
// first finds every write the record has seen, whichever goroutine made it
// and whether or not the record has told its listener yet. Hearing the
// record is only a prompt to pull: the Follower takes the run's lock itself
// and follows, so no reaction runs inside a writer's lock.
//
// Follower implements record.Listener. Its methods other than the
// listener's run under the run's lock.
type Follower struct {
	run   *record.Run
	lock  sync.Locker
	react Reactions

	// pulled and settles are scratch: the Tasks the record reported
	// changed, and the ones among them whose settle is owed an answer.
	pulled  []record.TaskID
	settles []settle
	// following is set while Follow runs, so a reaction that reads the
	// record does not follow it again.
	following bool
	// answered counts the settles answered. It is atomic so the listener
	// can see, without the lock, that a settle it hears was answered.
	answered atomic.Uint64
}

// Reactions are what a run's projections owe a change to a Task.
type Reactions interface {
	// Changed follows one write to the Task id and reports the state it
	// left the Task in, and whether a settle of it is now owed an answer:
	// one answer per Task, however often the record tells of it.
	Changed(id record.TaskID) (to record.State, settled bool)
	// Settled answers the Task id's settle into to.
	Settled(id record.TaskID, to record.State)
}

// settle is a Task whose settle is owed an answer.
type settle struct {
	id record.TaskID
	to record.State
}

// NewFollower follows run under lock, answering with react.
func NewFollower(run *record.Run, lock sync.Locker, react Reactions) *Follower {
	return &Follower{run: run, lock: lock, react: react}
}

// TaskChanged has nothing to do: a change waits in the log for the next pull.
func (*Follower) TaskChanged(record.TaskID, record.State, record.State) {}

// TaskSettled follows the record, so a settle is answered promptly even
// when nothing reads. Most settles happen inside a locked section that
// already followed the record, so the counts prove the answer without
// taking the lock: the record's count is read first, so an answer that
// caught up to it covers every settle written before.
func (f *Follower) TaskSettled(record.TaskID, record.State, record.State) {
	if written := f.run.CountSettled(); f.answered.Load() >= written {
		return
	}
	f.lock.Lock()
	defer f.lock.Unlock()
	f.Follow()
}

// Follow pulls the record, then answers each settle it owes. A call made
// while one is running does nothing.
func (f *Follower) Follow() {
	if f.following {
		return
	}
	f.following = true
	defer func() { f.following = false }()
	f.Pull()
	for i := range f.settles {
		f.react.Settled(f.settles[i].id, f.settles[i].to)
		f.settles[i] = settle{}
		f.answered.Add(1)
	}
	f.settles = f.settles[:0]
}

// Pull follows every write the record has seen and notes the settles owed
// an answer. It answers nothing, so a read may pull in the middle of
// building a frame.
func (f *Follower) Pull() {
	f.pulled = f.run.TakeChanged(f.pulled[:0])
	for _, id := range f.pulled {
		if to, settled := f.react.Changed(id); settled {
			f.settles = append(f.settles, settle{id: id, to: to})
		}
	}
}
