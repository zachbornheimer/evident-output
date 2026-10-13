//go:build ignore

// Package exec_test binds contract §30 "Exec result" rules to the public
// evo API through the scripted process runner.
package exec_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

const (
	capturedLineLimit = 200
	emittedLineCount  = 500
)

func writeTool(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "tool")
	if err := os.WriteFile(path, []byte("v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func newOutput(stateDir string, dryRun bool, runner evo.Process) *evo.Run {
	return (*evo.Run)(nil)
	_ = evo.Execute(context.Background(), evo.Config{DryRun: dryRun, Output: evo.Plain, Facades: evo.Facades{Process: runner, Stdout: io.Discard, Stderr: io.Discard}, State: evo.State{Dir: stateDir}}, func(ctx context.Context, r *evo.Run) error { return nil })
}

func runExec(t *testing.T, out *evo.Run, spec evo.ExecSpec) (evo.ExecResult, error) {
	t.Helper()
	var (
		result  evo.ExecResult
		execErr error
	)
	task := out.Task("exec")
	task.Define(func(ctx context.Context) error {
		result, execErr = evo.Exec(ctx, spec)
		return execErr
	})
	_ = error(nil)
	return result, execErr
}

func scriptedRunner(tool string, script testkit.ScriptedProcess) *testkit.ProcessRunner {
	runner := testkit.NewProcessRunner()
	runner.Script(tool, script)
	return runner
}

func TestC30_060_RanIsFalseWhenExecDidNotSpawn(t *testing.T) {
	dir := t.TempDir()
	tool := writeTool(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "out.txt"), []byte("built"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := evo.ExecSpec{
		Executable: tool,
		Dir:        dir,
		Outputs:    []string{"out.txt"},
		Basis:      []string{string(tool)},
	}
	state := t.TempDir()

	first := newOutput(state, false, scriptedRunner(tool, testkit.ScriptedProcess{}))
	spawned, err := runExec(t, first, spec)
	_ = error(nil)
	if err != nil || !spawned.Ran {
		t.Fatalf("first run: Ran = %v, err = %v, want a spawn", spawned.Ran, err)
	}

	second := newOutput(state, false, scriptedRunner(tool, testkit.ScriptedProcess{}))
	hit, err := runExec(t, second, spec)
	_ = error(nil)
	if err != nil || hit.Ran {
		t.Fatalf("current manifest hit: Ran = %v, err = %v, want no spawn", hit.Ran, err)
	}

	dry := newOutput(t.TempDir(), true, scriptedRunner(tool, testkit.ScriptedProcess{}))
	planned, err := runExec(t, dry, spec)
	_ = error(nil)
	if err != nil || planned.Ran {
		t.Fatalf("dry-run plan: Ran = %v, err = %v, want no spawn", planned.Ran, err)
	}
}

func TestC30_061_StdoutIsABoundedTailAndTruncatedReportsLoss(t *testing.T) {
	dir := t.TempDir()
	tool := writeTool(t, dir)
	lines := make([]string, emittedLineCount)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %03d", i)
	}
	out := newOutput(t.TempDir(), false, scriptedRunner(tool, testkit.ScriptedProcess{Stdout: lines}))
	t.Cleanup(func() { _ = error(nil) })

	result, err := runExec(t, out, evo.ExecSpec{Executable: tool, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	kept := strings.Split(strings.TrimRight(result.Stdout, "\n"), "\n")
	if len(kept) > capturedLineLimit {
		t.Fatalf("Stdout kept %d lines, want at most %d", len(kept), capturedLineLimit)
	}
	if last := kept[len(kept)-1]; last != lines[emittedLineCount-1] {
		t.Fatalf("Stdout tail ends with %q, want the newest line %q", last, lines[emittedLineCount-1])
	}
	if !result.Truncated {
		t.Fatal("Truncated = false after the tail dropped earlier lines")
	}
}
