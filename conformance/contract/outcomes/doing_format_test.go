package outcomes_test

import (
	"context"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestC02_028_DoingFormatsItsVariadicArguments(t *testing.T) {
	var during evo.TaskSnapshot
	concluded(t, func(task *evo.TaskHandle) {
		task.Define(func(context.Context) error {
			task.Doing("syncing %s (%d of %d)", "widget", 3, 10)
			during = task.Snapshot()
			return nil
		})
	})
	if want := "syncing widget (3 of 10)"; during.Phase != want {
		t.Fatalf("activity = %q, want %q", during.Phase, want)
	}
}
