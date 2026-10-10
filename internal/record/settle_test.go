package record

import "testing"

func TestSettleIsHeardAsASettleInAnyEndingState(t *testing.T) {
	for _, ending := range []EntityState{Done, Failed, Incomplete} {
		run := NewRun()
		task := run.NewTask(TaskInit{ID: "task_1", State: Pending})
		heard := &callbackListener{}
		run.SetListener(heard)

		task.Settle(ending)

		if len(heard.settles) != 1 || heard.settles[0] != [2]EntityState{Pending, ending} || !task.IsSettled() {
			t.Errorf("settling %s: heard settles %v, settled %t, want one Pending to %[1]s", ending, heard.settles, task.IsSettled())
		}
	}
}

func TestTransitionIsNotASettle(t *testing.T) {
	run := NewRun()
	task := run.NewTask(TaskInit{ID: "task_1", State: Pending})
	heard := &callbackListener{}
	run.SetListener(heard)

	task.Transition(Running)

	if len(heard.settles) != 0 || task.IsSettled() {
		t.Errorf("a Transition was heard as a settle: %v", heard.settles)
	}
}
