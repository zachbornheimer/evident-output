package record

import (
	"sync"
	"testing"
)

// callbackListener records what it heard and runs probe on every callback, so
// a test can prove the record's lock was free while it was called.
type callbackListener struct {
	mu      sync.Mutex
	tasks   []TaskID
	moves   [][2]EntityState
	events  []Event
	onEvent func()
	onTask  func()
}

func (l *callbackListener) TaskChanged(id TaskID, from, to EntityState) {
	if l.onTask != nil {
		l.onTask()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tasks = append(l.tasks, id)
	l.moves = append(l.moves, [2]EntityState{from, to})
}

func (l *callbackListener) EventAppended(e Event) {
	if l.onEvent != nil {
		l.onEvent()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, e)
}

func TestListenerHearsEveryTaskWriteByID(t *testing.T) {
	run := NewRun()
	task := run.NewTask(TaskInit{ID: "task_1"})
	heard := &callbackListener{}
	run.SetListener(heard)

	task.SetPhase("working")
	task.AppendFact(Fact{})
	task.Transition(Running)

	if len(heard.tasks) != 3 {
		t.Fatalf("heard %d task changes, want 3: %v", len(heard.tasks), heard.tasks)
	}
	for _, id := range heard.tasks {
		if id != "task_1" || id != task.ID() {
			t.Errorf("heard %q, want task_1", id)
		}
	}
}

func TestListenerHearsOnlyWritesThatChangedSomething(t *testing.T) {
	run := NewRun()
	task := run.NewTask(TaskInit{ID: "task_1", State: Running})
	task.SetPhase("same")
	heard := &callbackListener{}
	run.SetListener(heard)

	task.SetPhaseIfChanged("same")
	task.ApplyProgress(-1, 0, Determinate)
	if len(heard.tasks) != 0 {
		t.Fatalf("heard %v for writes that changed nothing", heard.tasks)
	}
	task.SetPhaseIfChanged("different")
	task.ApplyProgress(1, 2, Determinate)
	if len(heard.tasks) != 2 {
		t.Errorf("heard %d changes for two real writes, want 2", len(heard.tasks))
	}
}

func TestListenerHearsTheStampedEvent(t *testing.T) {
	run := NewRun()
	heard := &callbackListener{}
	run.SetListener(heard)

	stamped := run.AppendEvent(Event{Type: "task.declared", EntityID: "task_1"}, 0)

	if len(heard.events) != 1 || heard.events[0].Sequence != stamped.Sequence || stamped.Sequence == 0 {
		t.Errorf("heard %+v, want the stamped %+v", heard.events, stamped)
	}
}

func TestListenerRunsAfterTheRecordLockIsReleased(t *testing.T) {
	run := NewRun()
	task := run.NewTask(TaskInit{ID: "task_1"})
	reads := 0
	heard := &callbackListener{}
	heard.onTask = func() { _ = task.State(); reads++ }
	heard.onEvent = func() { _ = run.Events(0); reads++ }
	run.SetListener(heard)

	task.SetSummary("done")
	run.AppendEvent(Event{Type: "x"}, 0)

	if reads != 2 {
		t.Errorf("listener read the record %d times, want 2 (a held lock would have deadlocked)", reads)
	}
}

func TestSetListenerNilStopsListening(t *testing.T) {
	run := NewRun()
	task := run.NewTask(TaskInit{ID: "task_1"})
	heard := &callbackListener{}
	run.SetListener(heard)
	run.SetListener(nil)

	task.SetSummary("quiet")
	run.AppendEvent(Event{Type: "x"}, 0)

	if len(heard.tasks)+len(heard.events) != 0 {
		t.Errorf("a removed listener heard %v and %v", heard.tasks, heard.events)
	}
}
