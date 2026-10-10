package record

import "testing"

func TestListenerHearsTheStateATransitionLeftAndEntered(t *testing.T) {
	run := NewRun()
	task := run.NewTask(TaskInit{ID: "task_1", State: Pending})
	heard := &callbackListener{}
	run.SetListener(heard)

	task.Transition(Running)
	task.SetPhase("working")

	want := [][2]EntityState{{Pending, Running}, {Running, Running}}
	if len(heard.moves) != len(want) || heard.moves[0] != want[0] || heard.moves[1] != want[1] {
		t.Errorf("heard moves %v, want %v", heard.moves, want)
	}
}

func TestHoldKeepsNotificationsBackUntilTheLastRelease(t *testing.T) {
	run := NewRun()
	task := run.NewTask(TaskInit{ID: "task_1"})
	heard := &callbackListener{}
	run.SetListener(heard)

	run.Hold()
	run.Hold()
	task.SetPhase("one")
	run.AppendEvent(Event{Type: "x"}, 0)
	run.Release()
	if len(heard.tasks)+len(heard.events) != 0 {
		t.Fatalf("heard %v and %v while a Hold was open", heard.tasks, heard.events)
	}
	run.Release()

	if len(heard.tasks) != 1 || len(heard.events) != 1 {
		t.Errorf("after the last Release heard %v and %v, want one of each", heard.tasks, heard.events)
	}
}

func TestReleaseDeliversInTheOrderTheWritesHappened(t *testing.T) {
	run := NewRun()
	task := run.NewTask(TaskInit{ID: "task_1", State: Pending})
	heard := &callbackListener{}
	run.SetListener(heard)

	run.Hold()
	task.Transition(Running)
	task.Transition(Done)
	run.Release()

	want := [][2]EntityState{{Pending, Running}, {Running, Done}}
	if len(heard.moves) != 2 || heard.moves[0] != want[0] || heard.moves[1] != want[1] {
		t.Errorf("heard %v, want %v", heard.moves, want)
	}
}

func TestListenerMayHoldAndWriteWhileItIsBeingTold(t *testing.T) {
	run := NewRun()
	task := run.NewTask(TaskInit{ID: "task_1"})
	heard := &callbackListener{}
	wrote := false
	heard.onTask = func() {
		if wrote {
			return
		}
		wrote = true
		run.Hold()
		defer run.Release()
		task.SetSummary("from the listener")
	}
	run.SetListener(heard)

	task.SetPhase("first")

	if len(heard.tasks) != 2 {
		t.Errorf("heard %d changes, want the write and the listener's own write", len(heard.tasks))
	}
}

func TestReleaseWithoutAHoldChangesNothing(t *testing.T) {
	run := NewRun()
	task := run.NewTask(TaskInit{ID: "task_1"})
	heard := &callbackListener{}
	run.SetListener(heard)

	run.Release()
	task.SetPhase("still told at once")

	if len(heard.tasks) != 1 {
		t.Errorf("heard %d changes, want 1", len(heard.tasks))
	}
}
