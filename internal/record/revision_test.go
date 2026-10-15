package record

import "testing"

func TestRevisionMovesWithEveryWriteAndOnlyWrites(t *testing.T) {
	run := NewRun()
	task := run.NewTask(TaskInit{ID: "task_1", State: Pending})
	start := run.Revision()

	_ = task.State()
	_ = task.Truth()
	if run.Revision() != start {
		t.Fatalf("reads moved the revision from %d to %d", start, run.Revision())
	}

	task.SetPhase("one")
	run.TakeChanged(nil)
	task.SetPhase("two")

	if got := run.Revision(); got != start+2 {
		t.Errorf("revision = %d after two writes, want %d, even though the second Task was already taken", got, start+2)
	}
}
