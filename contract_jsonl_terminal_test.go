package evo_test

import (
	"context"
	"errors"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestJSONLEmitsTerminalEventForEveryTask pins CONTRACT.md §6: a JSONL run
// emits exactly one task.finished for every Task it declared, including one
// that never started because a predecessor failed.
func TestJSONLEmitsTerminalEventForEveryTask(t *testing.T) {
	cases := map[string]evo.RunFunc{
		"After dependent of a failed Task": func(_ context.Context, r *evo.Run) error {
			build := r.Task("build").Define(func(context.Context) error { return errors.New("compile error") })
			r.Task("dependent").After(build).Define(func(context.Context) error { return nil })
			return nil
		},
		"later steps of a failed Sequence": func(_ context.Context, r *evo.Run) error {
			seq := r.Sequence("steps")
			seq.Task("first").Define(func(context.Context) error { return errors.New("broke") })
			seq.Task("second").Define(func(context.Context) error { return nil })
			seq.Task("third").Define(func(context.Context) error { return nil })
			return nil
		},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			run := executeContract(context.Background(), t, evo.Config{Name: "terminal events", Output: evo.JSONL}, body)
			events := runEvents(t, run.Stdout)
			finished := map[any]int{}
			for _, event := range eventsOfType(events, "task.finished") {
				finished[event["entity_id"]]++
			}
			for _, declared := range eventsOfType(events, "task.declared") {
				id := declared["entity_id"]
				if n := finished[id]; n != 1 {
					t.Errorf("task %v (%v) has %d task.finished events, want 1", id, payload(declared)["name"], n)
				}
			}
		})
	}
}
