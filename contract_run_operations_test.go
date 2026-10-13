package evo_test

import (
	"context"
	"path/filepath"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestRunDocumentListsOperationsAndBasis pins CONTRACT.md §8: a Task that
// wrote a File lists the operation, the basis it read and the resource it
// tracks, each drawn from the freshness record rather than left empty.
func TestRunDocumentListsOperationsAndBasis(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "in.txt")
	output := filepath.Join(dir, "out.txt")
	run := executeContract(context.Background(), t,
		evo.Config{Name: "operations", Output: evo.JSON, State: evo.State{Dir: t.TempDir(), AppID: "contract"}},
		func(_ context.Context, r *evo.Run) error {
			r.Task("writer").Define(func(ctx context.Context) error {
				if err := evo.File(ctx, evo.FileSpec{Path: input, Contents: []byte("input\n")}); err != nil {
					return err
				}
				return evo.File(ctx, evo.FileSpec{Path: output, Contents: []byte("out\n"), Inputs: []string{input}})
			})
			r.Task("idle").Define(func(context.Context) error { return nil })
			return nil
		})
	tasks := map[string]map[string]any{}
	for _, task := range documentTasks(t, runDocument(t, run.Stdout)) {
		name, _ := task["name"].(string)
		tasks[name] = task
	}

	writer := tasks["writer"]
	operations, _ := writer["operations"].([]any)
	if len(operations) != 2 {
		t.Fatalf("writer operations = %v, want one per File call", writer["operations"])
	}
	if op, _ := operations[0].(map[string]any); op["kind"] != "file" || op["executed"] != true || op["current"] != true {
		t.Errorf("writer operations[0] = %v, want an executed, current file operation", operations[0])
	}
	basis, _ := writer["basis"].([]any)
	if len(basis) == 0 {
		t.Errorf("writer basis = %v, want the input File read", writer["basis"])
	}
	resources, _ := writer["tracked_resources"].([]any)
	paths := map[any]bool{}
	for _, item := range resources {
		resource, _ := item.(map[string]any)
		paths[resource["path"]] = true
	}
	if !paths[input] || !paths[output] {
		t.Errorf("writer tracked_resources = %v, want both written files", writer["tracked_resources"])
	}
	for _, key := range []string{"operations", "basis", "tracked_resources"} {
		if list, _ := tasks["idle"][key].([]any); len(list) != 0 {
			t.Errorf("idle: %s = %v, want empty for a Task that touched nothing", key, list)
		}
	}
}
