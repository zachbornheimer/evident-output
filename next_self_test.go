package evo_test

import (
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestTaskHandle_NextSelf_UsesOwnIdentity is I6: NextSelf attaches a
// self-referencing remedy command without the caller restating its own
// binary name — it resolves the executable from the same identity source
// as Confirm's PolicyFlag (Config.Title, or the executable-basename
// fallback).
func TestTaskHandle_NextSelf_UsesOwnIdentity(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("clean-repo")}})

	task := out.Task("dry run")
	task.NextSelf("--apply")
	task.Done()

	item := out.Snapshot().Tasks[0]
	if len(item.Actions) == 0 || item.Actions[0].Command == nil {
		t.Fatalf("actions = %+v, want a Command action", item.Actions)
	}
	cmd := item.Actions[0].Command
	if cmd.Executable != "clean-repo" || len(cmd.Args) != 1 || cmd.Args[0] != "--apply" {
		t.Fatalf("command = %+v, want executable %q with arg %q", cmd, "clean-repo", "--apply")
	}
}
