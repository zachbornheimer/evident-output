package evo_test

import (
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestOutput_PipedStdoutNeverInfersMachineOutput pins CONTRACT.md §6: a
// captured (non-terminal) stdout with no EVO_OUTPUT stays human output.
func TestOutput_PipedStdoutNeverInfersMachineOutput(t *testing.T) {
	t.Setenv("EVO_OUTPUT", "")
	run := executeContract(context.Background(), t, evo.Config{Name: "piped"}, func(_ context.Context, r *evo.Run) error {
		r.Task("work").Define(func(context.Context) error { return nil })
		return nil
	})
	if strings.HasPrefix(strings.TrimSpace(run.Stdout), "{") {
		t.Fatalf("piped stdout became machine output unasked:\n%s", run.Stdout)
	}
}
