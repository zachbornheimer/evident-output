//go:build v06acceptance

// Package exec_test holds increment 3's behavioral fixtures: Exec's skip
// protocol, capture/activity/redaction, cancellation, dry-run, and the
// spec §64 multi-stage pipeline scenarios (normalize → compile). Moved out
// of ../../api/pending (removed — increment 3 is the last pending tranche)
// now that evo.Exec exists.
package exec_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// isolated builds an Output against stateDir/runner without registering a
// t.Cleanup close: a test that opens more than one Output sharing one
// StateDir (simulating separate process runs against one manifest) must
// close each one explicitly before opening the next — the manifest Store
// holds an exclusive file lock on StateDir for as long as its Output stays
// open, so two live Outputs on the same StateDir deadlock the second Open.
func isolated(stateDir string, runner evo.ProcessRunner) *evo.Output {
	return evo.Init(evo.Config{Isolated: true, StateDir: stateDir, ProcessRunner: runner, Stdout: os.Stdout, Stderr: os.Stderr})
}

func execFixture(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func runExecTask(out *evo.Output, name string, spec evo.ExecSpec) error {
	var execErr error
	task := out.Task(name)
	task.Define(func(ctx context.Context) error {
		execErr = evo.Exec(ctx, spec)
		return execErr
	})
	_ = task.Wait()
	return execErr
}

func TestV06ExecSkipsSecondRunWhenOutputsUnchanged(t *testing.T) {
	state := t.TempDir()
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	outPath := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(outPath, []byte("built"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := evo.ExecSpec{Executable: tool, Dir: dir, Outputs: []string{"out.txt"}}

	first := testkit.NewProcessRunner()
	first.Script(tool, testkit.ScriptedProcess{ExitCode: 0})
	firstOut := isolated(state, first)
	if err := runExecTask(firstOut, "build", spec); err != nil {
		t.Fatalf("first run: %v", err)
	}
	_ = firstOut.Close()

	second := testkit.NewProcessRunner()
	second.Script(tool, testkit.ScriptedProcess{ExitCode: 0})
	secondOut := isolated(state, second)
	t.Cleanup(func() { _ = secondOut.Close() })
	if err := runExecTask(secondOut, "build", spec); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if calls := second.Calls(); len(calls) != 0 {
		t.Fatalf("second run spawned %d times, want 0 (unchanged Outputs must skip)", len(calls))
	}
}

func TestV06ExecMissingOutputAfterSuccessFails(t *testing.T) {
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	runner := testkit.NewProcessRunner()
	runner.Script(tool, testkit.ScriptedProcess{ExitCode: 0})
	spec := evo.ExecSpec{Executable: tool, Dir: dir, Outputs: []string{"never-written.txt"}}

	out := isolated(t.TempDir(), runner)
	t.Cleanup(func() { _ = out.Close() })
	err := runExecTask(out, "missing", spec)
	if !errors.Is(err, evo.ErrExecOutputMissingAfterSuccess) {
		t.Fatalf("err = %v, want ErrExecOutputMissingAfterSuccess", err)
	}
}

func TestV06ExecNonzeroExitFailsAndRetainsTail(t *testing.T) {
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	runner := testkit.NewProcessRunner()
	runner.Script(tool, testkit.ScriptedProcess{ExitCode: 1, Stderr: []string{"compile error: line 3"}})

	out := isolated(t.TempDir(), runner)
	t.Cleanup(func() { _ = out.Close() })
	err := runExecTask(out, "fails", evo.ExecSpec{Executable: tool})
	if !errors.Is(err, evo.ErrExecNonzeroExit) {
		t.Fatalf("err = %v, want ErrExecNonzeroExit", err)
	}
}

func TestV06ExecDryRunNeverSpawns(t *testing.T) {
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	runner := testkit.NewProcessRunner()
	runner.Script(tool, testkit.ScriptedProcess{ExitCode: 0})
	out := evo.Init(evo.Config{Isolated: true, DryRun: true, StateDir: t.TempDir(), ProcessRunner: runner})
	t.Cleanup(func() { _ = out.Close() })

	if err := runExecTask(out, "dry", evo.ExecSpec{Executable: tool, Dir: dir, Outputs: []string{"out.txt"}}); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if calls := runner.Calls(); len(calls) != 0 {
		t.Fatal("dry-run Exec must never spawn")
	}
}

func TestV06ExecCancellationKillsProcess(t *testing.T) {
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	gate := make(chan struct{}) // never closed: only ctx cancellation may release Run
	runner := testkit.NewProcessRunner()
	runner.Script(tool, testkit.ScriptedProcess{ExitCode: 0, Gate: gate})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := isolated(t.TempDir(), runner)
	t.Cleanup(func() { _ = out.Close() })
	out.Run(ctx, func(context.Context) error {
		task := out.Task("cancelled")
		task.Define(func(taskCtx context.Context) error { return evo.Exec(taskCtx, evo.ExecSpec{Executable: tool}) })
		return nil
	})
	if out.Err() == nil {
		t.Fatal("a cancelled Exec must report a non-nil run error")
	}
}

func TestV06ExecCapturedLineBecomesActivityAndRedactsSecrets(t *testing.T) {
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	runner := testkit.NewProcessRunner()
	runner.Script(tool, testkit.ScriptedProcess{ExitCode: 1, Stdout: []string{"token=super-secret"}})

	out := evo.Init(evo.Config{
		Isolated: true, StateDir: t.TempDir(), ProcessRunner: runner,
		Redactor: secretRedactor{secret: "super-secret"},
	})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("narrated")
	var phaseDuringRun string
	task.Define(func(ctx context.Context) error {
		err := evo.Exec(ctx, evo.ExecSpec{Executable: tool})
		phaseDuringRun = task.Snapshot().Phase
		return err
	})
	_ = task.Wait()
	if phaseDuringRun != "token=[redacted]" {
		t.Fatalf("task phase = %q, want the redacted captured line", phaseDuringRun)
	}
	for _, p := range task.Snapshot().Problems {
		if strings.Contains(p.EvidenceTail, "super-secret") {
			t.Fatalf("evidence tail leaked the secret: %q", p.EvidenceTail)
		}
	}
}

type secretRedactor struct{ secret string }

func (r secretRedactor) RedactString(s string) string {
	return strings.ReplaceAll(s, r.secret, "[redacted]")
}
