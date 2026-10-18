package evo_test

import (
	"context"
	"errors"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
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

// TestJSONLNeverEmitsBuilderGates pins CONTRACT.md §7: a container builder is
// the library's own scheduling, not a declared Task, so evo.event names only
// the Tasks and collections the program declared, and every task.finished
// answers a task.declared.
func TestJSONLNeverEmitsBuilderGates(t *testing.T) {
	run := executeContract(context.Background(), t, evo.Config{Name: "builder gates", Output: evo.JSONL},
		func(_ context.Context, r *evo.Run) error {
			prepare := r.Task("prepare").Define(func(context.Context) error { return nil })
			r.Group("items").After(prepare).Build(func(g *evo.Group) {
				for _, name := range []string{"a", "b"} {
					g.Task(name).Define(func(context.Context) error { return nil })
				}
			})
			return nil
		})
	events := runEvents(t, run.Stdout)
	declared := map[any]bool{}
	for _, typ := range []string{"task.declared", "collection.declared"} {
		for _, event := range eventsOfType(events, typ) {
			declared[event["entity_id"]] = true
		}
	}
	for _, event := range events {
		if id, named := event["entity_id"]; named && id != "" && !declared[id] {
			t.Errorf("seq %v %v names %v, which the program never declared", event["seq"], event["type"], id)
		}
	}
}
