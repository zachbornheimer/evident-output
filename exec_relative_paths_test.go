package evo

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExecRelativePathsResolveAgainstDir pins CONTRACT.md section 3: a relative
// ExecSpec.Outputs entry resolves against ExecSpec.Dir, while a relative
// ExecSpec.Inputs entry resolves against the run workspace, like FileSpec.Path.
func TestExecRelativePathsResolveAgainstDir(t *testing.T) {
	workspace, dir, state := t.TempDir(), t.TempDir(), t.TempDir()
	t.Chdir(workspace)
	tool := execFixture(t, dir, "tool", "v1")
	input := filepath.Join(workspace, "in.txt")
	output := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(input, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	spec := ExecSpec{executable: tool, Dir: dir, Inputs: []string{"in.txt"}, Outputs: []string{"out.txt"}}
	// The fake runner spawns nothing; this stands in for the child's own write.
	produce := func() {
		if err := os.WriteFile(output, []byte("built"), 0o600); err != nil {
			t.Error(err)
		}
	}
	run := func(state string, inRun func(*Run)) (spawns int) {
		runner := &scriptedRunner{onRun: produce}
		out := initRun(Config{State: State{Dir: state}, Facades: Facades{Process: runner}})
		defer func() { _ = out.close() }()
		if err := runExecTask(t, out, "build", spec); err != nil {
			t.Fatalf("Exec: %v", err)
		}
		if inRun != nil {
			inRun(out)
		}
		return len(runner.calls)
	}

	// A rival Run state keeps the deliberate conflict out of the freshness record below.
	run(t.TempDir(), func(out *Run) {
		// The claimed Output is the Dir-joined path: declaring it absolutely
		// from another Task conflicts with the relative claim.
		other := ExecSpec{executable: tool, Outputs: []string{output}}
		if err := runExecTask(t, out, "rival", other); err == nil {
			t.Error("absolute Dir-joined Output was not claimed by the relative Output")
		}
	})
	if got := run(state, nil); got != 1 {
		t.Fatalf("first run spawns = %d, want 1", got)
	}
	if got := run(state, nil); got != 0 {
		t.Fatalf("unchanged workspace Input and Dir Output: spawns = %d, want 0", got)
	}
	if err := os.WriteFile(input, []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := run(state, nil); got != 1 {
		t.Fatalf("changed workspace Input: spawns = %d, want 1 (Inputs resolve against the workspace)", got)
	}
}
