package evo

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
)

// scriptedRunner is a minimal in-package ProcessRunner fake — testkit's own
// fake (testkit.ProcessRunner) lives in a different module path and can't
// be imported here without an import cycle (testkit imports evo, evo
// imports engine); this package's own tests need the same scripting.
type scriptedRunner struct {
	exitCode int
	spawnErr error
	stdout   []string
	stderr   []string
	gate     chan struct{}
	calls    [][]string
	// onRun, when set, runs synchronously right after gate releases and
	// before Run returns — simulating a child that produces its declared
	// Output as its very last act (used to prove the freshness barrier
	// delivers the producer's *final* content to a waiting consumer).
	onRun func()
}

func (r *scriptedRunner) Run(ctx context.Context, argv []string, _ string, _ []string, stdout, stderr io.Writer) (int, error) {
	r.calls = append(r.calls, argv)
	if r.gate != nil {
		select {
		case <-r.gate:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if r.spawnErr != nil {
		return 0, r.spawnErr
	}
	if r.onRun != nil {
		r.onRun()
	}
	for _, l := range r.stdout {
		_, _ = stdout.Write([]byte(l + "\n"))
	}
	for _, l := range r.stderr {
		_, _ = stderr.Write([]byte(l + "\n"))
	}
	return r.exitCode, nil
}

// routedRunner dispatches to a different ProcessRunner per resolved
// executable path — Config carries exactly one ProcessRunner per Output, so
// a producer/consumer freshness test that needs independently controllable
// spawns for two different Tasks routes through this.
type routedRunner struct{ byPath map[string]processRunner }

func (r routedRunner) Run(ctx context.Context, argv []string, dir string, env []string, stdout, stderr io.Writer) (int, error) {
	return r.byPath[argv[0]].Run(ctx, argv, dir, env, stdout, stderr)
}

// execFixture builds a real (non-executed) stub file at dir/name and
// returns its path — Exec's definition fingerprint reads the executable's
// own content digest, so the file must exist even though scriptedRunner
// never actually execs it.
func execFixture(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// runExecTask runs one Task named name whose Define calls Exec(spec) once
// against out, waiting for it to settle, and returns Exec's own error.
func runExecTask(t *testing.T, out *Run, name string, spec ExecSpec) error {
	t.Helper()
	_, err := runExecTaskResult(t, out, name, spec)
	return err
}

// runExecTaskResult is runExecTask but also returns the ExecResult Exec
// itself produced, so callers can assert Ran/ExitCode/Stdout/Stderr/
// Truncated instead of only the error.
func runExecTaskResult(t *testing.T, out *Run, name string, spec ExecSpec) (ExecResult, error) {
	t.Helper()
	var execResult ExecResult
	var execErr error
	task := out.Task(name)
	task.Define(func(ctx context.Context) error {
		execResult, execErr = Exec(ctx, spec)
		return execErr
	})
	_ = task.wait()
	return execResult, execErr
}

func TestExecSpawnsWhenNoOutputsDeclared(t *testing.T) {
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	runner := &scriptedRunner{exitCode: 0}
	out := initRun(Config{State: State{Dir: t.TempDir()}, Facades: Facades{Process: runner}})
	t.Cleanup(func() { _ = out.close() })

	if err := runExecTask(t, out, "no-outputs", ExecSpec{executable: tool}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := runExecTask(t, out, "no-outputs-2", ExecSpec{executable: tool}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("spawn count = %d, want 2 (No Outputs must always run)", len(runner.calls))
	}
}

func TestExecSkipsSecondRunWhenOutputsUnchanged(t *testing.T) {
	state := t.TempDir()
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	outPath := filepath.Join(dir, "out.txt")

	firstRunner := &scriptedRunner{exitCode: 0, stdout: []string{"built"}}
	first := initRun(Config{State: State{Dir: state}, Facades: Facades{Process: firstRunner}})
	spec := ExecSpec{executable: tool, Outputs: []string{"out.txt"}, Dir: dir}
	// The fake runner does not itself write files; simulate the child's own
	// effect (declared Outputs are its contract, not evo's).
	if err := os.WriteFile(outPath, []byte("built"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runExecTask(t, first, "build", spec); err != nil {
		t.Fatalf("first run: %v", err)
	}
	_ = first.close()
	if len(firstRunner.calls) != 1 {
		t.Fatalf("first run spawn count = %d, want 1", len(firstRunner.calls))
	}

	secondRunner := &scriptedRunner{exitCode: 0}
	second := initRun(Config{State: State{Dir: state}, Facades: Facades{Process: secondRunner}})
	t.Cleanup(func() { _ = second.close() })
	result, err := runExecTaskResult(t, second, "build", spec)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(secondRunner.calls) != 0 {
		t.Fatalf("second run spawn count = %d, want 0 (unchanged Outputs must skip)", len(secondRunner.calls))
	}
	if result.Ran {
		t.Fatalf("result.Ran = true, want false (manifest-current skip must not spawn)")
	}
	if result.Stdout != "" || result.Stderr != "" {
		t.Fatalf("result = %+v, want empty streams on a skipped run", result)
	}
}

func TestExecRerunsWhenOutputDrifts(t *testing.T) {
	state := t.TempDir()
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	outPath := filepath.Join(dir, "out.txt")
	spec := ExecSpec{executable: tool, Outputs: []string{"out.txt"}, Dir: dir}

	if err := os.WriteFile(outPath, []byte("built"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := initRun(Config{State: State{Dir: state}, Facades: Facades{Process: &scriptedRunner{exitCode: 0}}})
	if err := runExecTask(t, first, "build", spec); err != nil {
		t.Fatalf("first run: %v", err)
	}
	_ = first.close()

	// External drift: someone edited the output outside Evo.
	if err := os.WriteFile(outPath, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &scriptedRunner{exitCode: 0}
	second := initRun(Config{State: State{Dir: state}, Facades: Facades{Process: runner}})
	t.Cleanup(func() { _ = second.close() })
	if err := runExecTask(t, second, "build", spec); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("output drift spawn count = %d, want 1", len(runner.calls))
	}
}

func TestExecMissingOutputAfterSuccessFails(t *testing.T) {
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	spec := ExecSpec{executable: tool, Outputs: []string{"never-written.txt"}, Dir: dir}
	out := initRun(Config{State: State{Dir: t.TempDir()}, Facades: Facades{Process: &scriptedRunner{exitCode: 0}}})
	t.Cleanup(func() { _ = out.close() })

	result, err := runExecTaskResult(t, out, "missing-output", spec)
	if !errors.Is(err, change.ErrExecOutputMissingAfterSuccess) {
		t.Fatalf("err = %v, want ErrExecOutputMissingAfterSuccess", err)
	}
	if !result.Ran {
		t.Fatalf("result.Ran = false, want true (the child did reach a terminal zero exit)")
	}
	if result.ExitCode != 0 {
		t.Fatalf("result.ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestExecNonzeroExitFailsAndCommitsNoRecord(t *testing.T) {
	state := t.TempDir()
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	outPath := filepath.Join(dir, "out.txt")
	spec := ExecSpec{executable: tool, Outputs: []string{"out.txt"}, Dir: dir}

	runner := &scriptedRunner{exitCode: 1, stderr: []string{"boom"}}
	out := initRun(Config{State: State{Dir: state}, Facades: Facades{Process: runner}})
	result, err := runExecTaskResult(t, out, "fails", spec)
	_ = out.close()
	if !errors.Is(err, ErrExecNonzeroExit) {
		t.Fatalf("err = %v, want ErrExecNonzeroExit", err)
	}
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Fatal("nonzero exit must not fabricate the declared output")
	}
	if !result.Ran {
		t.Fatal("result.Ran = false, want true (the child reached a terminal nonzero exit)")
	}
	if result.ExitCode != 1 {
		t.Fatalf("result.ExitCode = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "boom") {
		t.Fatalf("result.Stderr = %q, want the captured stderr", result.Stderr)
	}

	// A following run with a succeeding runner must still spawn — no
	// success record was committed for the failed attempt.
	if err := os.WriteFile(outPath, []byte("built"), 0o644); err != nil {
		t.Fatal(err)
	}
	nextRunner := &scriptedRunner{exitCode: 0}
	next := initRun(Config{State: State{Dir: state}, Facades: Facades{Process: nextRunner}})
	t.Cleanup(func() { _ = next.close() })
	if err := runExecTask(t, next, "fails", spec); err != nil {
		t.Fatalf("recovery run: %v", err)
	}
	if len(nextRunner.calls) != 1 {
		t.Fatal("a failed attempt must not have committed a success record")
	}
}

func TestExecDryRunNeverSpawns(t *testing.T) {
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	spec := ExecSpec{executable: tool, Outputs: []string{"out.txt"}, Dir: dir}
	runner := &scriptedRunner{exitCode: 0}
	out := initRun(Config{DryRun: true, State: State{Dir: t.TempDir()}, Facades: Facades{Process: runner}})
	t.Cleanup(func() { _ = out.close() })

	result, err := runExecTaskResult(t, out, "dry", spec)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatal("dry-run Exec must never spawn")
	}
	if result.Ran {
		t.Fatal("result.Ran = true, want false (dry-run must never spawn)")
	}
	if result.Stdout != "" || result.Stderr != "" {
		t.Fatalf("result = %+v, want empty streams on a dry run", result)
	}
}

func TestExecCancellationKillsProcess(t *testing.T) {
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	spec := ExecSpec{executable: tool}
	gate := make(chan struct{}) // never closed: only ctx cancellation may release Run
	runner := &scriptedRunner{exitCode: 0, gate: gate}
	out := initRun(Config{State: State{Dir: t.TempDir()}, Facades: Facades{Process: runner}})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	task := out.Task("cancel-me")
	task.Define(func(taskCtx context.Context) error {
		_, err := Exec(taskCtx, spec)
		done <- err
		return err
	})
	// Give the scheduler a moment to enter Define and reach the gated Run.
	time.Sleep(20 * time.Millisecond)
	cancel()
	_ = out.run(ctx, func(ctx context.Context, r *Run) error { return nil })

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled Exec must report a non-nil error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not release the gated process")
	}
}

func TestExecBasisDriftForcesRespawn(t *testing.T) {
	state := t.TempDir()
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	outPath := filepath.Join(dir, "out.txt")

	if err := os.WriteFile(outPath, []byte("built"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec1 := ExecSpec{
		executable: tool, Outputs: []string{"out.txt"}, Dir: dir,
		basis: []freshness.Fingerprint{freshness.Value("input", 1)},
	}
	first := initRun(Config{State: State{Dir: state}, Facades: Facades{Process: &scriptedRunner{exitCode: 0}}})
	if err := runExecTask(t, first, "build", spec1); err != nil {
		t.Fatalf("first run: %v", err)
	}
	_ = first.close()

	spec2 := spec1
	spec2.basis = []freshness.Fingerprint{freshness.Value("input", 2)}
	runner := &scriptedRunner{exitCode: 0}
	second := initRun(Config{State: State{Dir: state}, Facades: Facades{Process: runner}})
	t.Cleanup(func() { _ = second.close() })
	if err := runExecTask(t, second, "build", spec2); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatal("Basis drift must force a respawn")
	}
}

func TestExecCapturedLineBecomesActivity(t *testing.T) {
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	runner := &scriptedRunner{exitCode: 0, stdout: []string{"step one", "step two"}}
	out := initRun(Config{State: State{Dir: t.TempDir()}, Facades: Facades{Process: runner}})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("narrated")
	var lastPhase string
	task.Define(func(ctx context.Context) error {
		_, err := Exec(ctx, ExecSpec{executable: tool})
		lastPhase = task.snapshot().Phase
		return err
	})
	_ = task.wait()
	if lastPhase != "step two" {
		t.Fatalf("task phase = %q, want the last captured line", lastPhase)
	}
}

func TestExecCapturedSecretIsRedactedBeforeRetention(t *testing.T) {
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	runner := &scriptedRunner{exitCode: 1, stdout: []string{"token=super-secret-value"}}
	out := initRun(Config{State: State{Dir: t.TempDir()}, Facades: Facades{Process: runner, Redactor: secretRedactor{secret: "super-secret-value"}}})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("secret")
	var tail string
	var result ExecResult
	task.Define(func(ctx context.Context) error {
		res, err := Exec(ctx, ExecSpec{executable: tool})
		result = res
		tail = task.captureForTest().Text()
		return err
	})
	_ = task.wait()
	if want := "super-secret-value"; strings.Contains(tail, want) {
		t.Fatalf("evidence tail leaked the secret: %q", tail)
	}
	if strings.Contains(result.Stdout, "super-secret-value") {
		t.Fatalf("ExecResult.Stdout leaked the secret: %q", result.Stdout)
	}
	if !strings.Contains(result.Stdout, "[redacted]") {
		t.Fatalf("ExecResult.Stdout = %q, want a redaction marker", result.Stdout)
	}
	if !strings.Contains(tail, "[redacted]") {
		t.Fatalf("evidence tail = %q, want a redaction marker", tail)
	}
}

type secretRedactor struct{ secret string }

func (r secretRedactor) RedactString(s string) string {
	return strings.ReplaceAll(s, r.secret, "[redacted]")
}

// TestExecFreshnessBarrierWaitsForProducerThenConsumesFinalOutput proves
// spec §11.6's known-producer barrier itself, deterministically: once a
// producer has claimed a canonical Output (spec: claiming opens the gate
// immediately, before that operation's spawn even starts), a consumer's
// concurrent Basis fingerprint on that same path blocks until the producer
// settles, then observes its *final* written content — never an
// intermediate/missing state. Both Exec calls run concurrently from within
// one Task's Define (the Define scope keys off ctx, not goroutine) so the test
// controls ordering directly instead of racing the scheduler's own Task
// start order — the scheduler is deliberately not exercised here; a real
// pipeline still declares After()/Sequence for real ordering.
func TestExecFreshnessBarrierWaitsForProducerThenConsumesFinalOutput(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.json")
	normalizeTool := execFixture(t, dir, "normalize", "v1")
	compileTool := execFixture(t, dir, "compile", "v1")
	if err := os.WriteFile(filepath.Join(dir, "compiled.txt"), []byte("placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}

	gate := make(chan struct{})
	producerRunner := &scriptedRunner{exitCode: 0, gate: gate, onRun: func() {
		if err := os.WriteFile(schemaPath, []byte("final-schema"), 0o644); err != nil {
			t.Error(err)
		}
	}}
	consumerRunner := &scriptedRunner{exitCode: 0}
	router := routedRunner{byPath: map[string]processRunner{
		normalizeTool: producerRunner,
		compileTool:   consumerRunner,
	}}

	out := initRun(Config{State: State{Dir: t.TempDir()}, Facades: Facades{Process: router}})
	t.Cleanup(func() { _ = out.close() })

	consumerObservedSchema := make(chan string, 1)
	producerDone := make(chan struct{})
	task := out.Task("pipeline")
	task.Define(func(taskCtx context.Context) error {
		go func() {
			defer close(producerDone)
			_, _ = Exec(taskCtx, ExecSpec{executable: normalizeTool, Dir: dir, Outputs: []string{"schema.json"}})
		}()

		// Wait for the producer's claim (spec: opened at claim time, before
		// its spawn) so the consumer's check is guaranteed to find the gate
		// already open — proving the barrier, not luck, does the blocking.
		canonPath := filepath.Clean(schemaPath)
		for {
			if _, opened := out.graph.Barrier(canonPath); opened {
				break
			}
			time.Sleep(time.Millisecond)
		}

		go func() {
			time.Sleep(20 * time.Millisecond)
			close(gate)
		}()

		_, err := Exec(taskCtx, ExecSpec{
			executable: compileTool, Dir: dir,
			basis:   []freshness.Fingerprint{freshness.FSPath(schemaPath)},
			Outputs: []string{"compiled.txt"},
		})
		got, readErr := os.ReadFile(schemaPath)
		if readErr == nil {
			consumerObservedSchema <- string(got)
		} else {
			consumerObservedSchema <- ""
		}
		<-producerDone
		return err
	})
	_ = task.wait()

	select {
	case observed := <-consumerObservedSchema:
		if observed != "final-schema" {
			t.Fatalf("consumer observed schema.json = %q, want the producer's final content", observed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("consumer never observed schema.json")
	}
}

// TestExecProducerConflictFailsSecondClaim proves spec §8.3/§11.4: two
// Tasks in one Run both declaring the same canonical Output path is a
// conflict, reported at the second claim.
func TestExecProducerConflictFailsSecondClaim(t *testing.T) {
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	sharedPath := filepath.Join(dir, "shared.txt")
	if err := os.WriteFile(sharedPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := initRun(Config{State: State{Dir: t.TempDir()}, Facades: Facades{Process: &scriptedRunner{exitCode: 0}}})
	t.Cleanup(func() { _ = out.close() })

	first := out.Task("first")
	first.Define(func(ctx context.Context) error {
		_, err := Exec(ctx, ExecSpec{executable: tool, Dir: dir, Outputs: []string{"shared.txt"}})
		return err
	})
	_ = first.wait()

	var secondErr error
	second := out.Task("second")
	second.Define(func(ctx context.Context) error {
		_, secondErr = Exec(ctx, ExecSpec{executable: tool, Dir: dir, Outputs: []string{"shared.txt"}})
		return secondErr
	})
	_ = second.wait()

	if !errors.Is(secondErr, change.ErrFileConflictingProducer) {
		t.Fatalf("second claim err = %v, want ErrFileConflictingProducer", secondErr)
	}
}
