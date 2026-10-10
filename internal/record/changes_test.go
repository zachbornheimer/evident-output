package record

import (
	"slices"
	"testing"
)

func TestTakeChangedReportsEachWrittenTaskOnceInFirstWriteOrder(t *testing.T) {
	run := NewRun()
	first := run.NewTask(TaskInit{ID: "task_1"})
	second := run.NewTask(TaskInit{ID: "task_2"})
	run.NewTask(TaskInit{ID: "task_3"})

	second.SetPhase("working")
	first.Transition(Running)
	second.AppendFact(Fact{})

	got := run.TakeChanged(nil)
	if want := []TaskID{"task_2", "task_1"}; !slices.Equal(got, want) {
		t.Errorf("TakeChanged = %v, want %v", got, want)
	}
	if again := run.TakeChanged(nil); len(again) != 0 {
		t.Errorf("a second TakeChanged = %v, want nothing", again)
	}
}

func TestTakeChangedReportsATaskAgainAfterItWasTaken(t *testing.T) {
	run := NewRun()
	task := run.NewTask(TaskInit{ID: "task_1"})

	task.SetPhase("one")
	run.TakeChanged(nil)
	task.SetPhase("two")

	if got := run.TakeChanged(nil); !slices.Equal(got, []TaskID{"task_1"}) {
		t.Errorf("TakeChanged = %v, want task_1 again", got)
	}
}

func TestTakeChangedSkipsWritesThatChangedNothing(t *testing.T) {
	run := NewRun()
	task := run.NewTask(TaskInit{ID: "task_1", State: Running})
	task.SetPhase("same")
	run.TakeChanged(nil)

	task.SetPhaseIfChanged("same")
	task.ApplyProgress(-1, 0, Determinate)

	if got := run.TakeChanged(nil); len(got) != 0 {
		t.Errorf("TakeChanged = %v, want nothing for writes that changed nothing", got)
	}
}

// TestTakeChangedFindsAWriteTheListenerHasNotHeardYet proves the log is the
// exact source: while a Hold keeps the notification back, a reader that
// already sees the new state still finds the Task in the log.
func TestTakeChangedFindsAWriteTheListenerHasNotHeardYet(t *testing.T) {
	run := NewRun()
	task := run.NewTask(TaskInit{ID: "task_1", State: Pending})
	heard := &callbackListener{}
	run.SetListener(heard)

	run.Hold()
	task.Transition(Done)
	changed := run.TakeChanged(nil)
	heardInside := len(heard.tasks)
	run.Release()

	if !slices.Equal(changed, []TaskID{"task_1"}) || task.State() != Done {
		t.Errorf("TakeChanged = %v with the Task %s, want task_1 settled Done", changed, task.State())
	}
	if heardInside != 0 {
		t.Errorf("the listener heard %d changes while a Hold was open", heardInside)
	}
}
