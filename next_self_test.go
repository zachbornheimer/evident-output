package evo

import (
	"testing"
)

// TestTaskHandle_NextSelf_UsesOwnIdentity is I6: NextSelf attaches a
// self-referencing remedy command without the caller restating its own
// binary name — it resolves the executable from the same identity source
// as Confirm's PolicyFlag (Config.Title, or the executable-basename
// fallback).
func TestTaskHandle_NextSelf_UsesOwnIdentity(t *testing.T) {
	out := initRun(Config{Name: "clean-repo"})

	task := out.Task("dry run")
	task.Problem("rerun with --apply", out.nextSelfForTest("--apply"))
	succeed(task)

	item := out.snapshot().Tasks[0]
	if len(item.Problems) == 0 || len(item.Problems[0].Actions) == 0 || item.Problems[0].Actions[0].Command == nil {
		t.Fatalf("actions = %+v, want a Command action", item.Problems[0].Actions)
	}
	cmd := item.Problems[0].Actions[0].Command
	if cmd.Executable != "clean-repo" || len(cmd.Args) != 1 || cmd.Args[0] != "--apply" {
		t.Fatalf("command = %+v, want executable %q with arg %q", cmd, "clean-repo", "--apply")
	}
}
