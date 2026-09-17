package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/fingerprint"
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
	calls    []ProcessCommand
	// onRun, when set, runs synchronously right after gate releases and
	// before Run returns — simulating a child that produces its declared
	// Output as its very last act (used to prove the freshness barrier
	// delivers the producer's *final* content to a waiting consumer).
	onRun func()
}

func (r *scriptedRunner) Run(ctx context.Context, cmd ProcessCommand) (ProcessOutcome, error) {
	r.calls = append(r.calls, cmd)
	if r.gate != nil {
		select {
		case <-r.gate:
		case <-ctx.Done():
			return ProcessOutcome{}, ctx.Err()
		}
	}
	if ctx.Err() != nil {
		return ProcessOutcome{}, ctx.Err()
	}
	if r.spawnErr != nil {
		return ProcessOutcome{}, r.spawnErr
	}
	if r.onRun != nil {
		r.onRun()
	}
	for _, l := range r.stdout {
		_, _ = cmd.Stdout.Write([]byte(l + "\n"))
	}
	for _, l := range r.stderr {
		_, _ = cmd.Stderr.Write([]byte(l + "\n"))
	}
	return ProcessOutcome{ExitCode: r.exitCode}, nil
}

// routedRunner dispatches to a different ProcessRunner per resolved
// executable path — Config carries exactly one ProcessRunner per Output, so
// a producer/consumer freshness test that needs independently controllable
// spawns for two different Tasks routes through this.
type routedRunner struct{ byPath map[string]ProcessRunner }

func (r routedRunner) Run(ctx context.Context, cmd ProcessCommand) (ProcessOutcome, error) {
	return r.byPath[cmd.Path].Run(ctx, cmd)
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
func runExecTask(t *testing.T, out *Output, name string, spec ExecSpec) error {
	t.Helper()
	var execErr error
	task := out.Task(name)
	task.Define(func(ctx context.Context) error {
		execErr = Exec(ctx, spec)
		return execErr
	})
	_ = task.Wait()
	return execErr
}

func TestExecSpawnsWhenNoOutputsDeclared(t *testing.T) {
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	runner := &scriptedRunner{exitCode: 0}
	out := Init(Config{Isolated: true, StateDir: t.TempDir(), ProcessRunner: runner})
	t.Cleanup(func() { _ = out.Close() })

	if err := runExecTask(t, out, "no-outputs", ExecSpec{Executable: tool}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := runExecTask(t, out, "no-outputs-2", ExecSpec{Executable: tool}); err != nil {
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
	first := Init(Config{Isolated: true, StateDir: state, ProcessRunner: firstRunner})
	spec := ExecSpec{Executable: tool, Outputs: []string{"out.txt"}, Dir: dir}
	// The fake runner does not itself write files; simulate the child's own
	// effect (declared Outputs are its contract, not evo's).
	if err := os.WriteFile(outPath, []byte("built"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runExecTask(t, first, "build", spec); err != nil {
		t.Fatalf("first run: %v", err)
	}
	_ = first.Close()
	if len(firstRunner.calls) != 1 {
		t.Fatalf("first run spawn count = %d, want 1", len(firstRunner.calls))
	}

	secondRunner := &scriptedRunner{exitCode: 0}
	second := Init(Config{Isolated: true, StateDir: state, ProcessRunner: secondRunner})
	t.Cleanup(func() { _ = second.Close() })
	if err := runExecTask(t, second, "build", spec); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(secondRunner.calls) != 0 {
		t.Fatalf("second run spawn count = %d, want 0 (unchanged Outputs must skip)", len(secondRunner.calls))
	}
}

func TestExecRerunsWhenOutputDrifts(t *testing.T) {
	state := t.TempDir()
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	outPath := filepath.Join(dir, "out.txt")
	spec := ExecSpec{Executable: tool, Outputs: []string{"out.txt"}, Dir: dir}

	if err := os.WriteFile(outPath, []byte("built"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := Init(Config{Isolated: true, StateDir: state, ProcessRunner: &scriptedRunner{exitCode: 0}})
	if err := runExecTask(t, first, "build", spec); err != nil {
		t.Fatalf("first run: %v", err)
	}
	_ = first.Close()

	// External drift: someone edited the output outside Evo.
	if err := os.WriteFile(outPath, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &scriptedRunner{exitCode: 0}
	second := Init(Config{Isolated: true, StateDir: state, ProcessRunner: runner})
	t.Cleanup(func() { _ = second.Close() })
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
	spec := ExecSpec{Executable: tool, Outputs: []string{"never-written.txt"}, Dir: dir}
	out := Init(Config{Isolated: true, StateDir: t.TempDir(), ProcessRunner: &scriptedRunner{exitCode: 0}})
	t.Cleanup(func() { _ = out.Close() })

	err := runExecTask(t, out, "missing-output", spec)
	if !errors.Is(err, ErrExecOutputMissingAfterSuccess) {
		t.Fatalf("err = %v, want ErrExecOutputMissingAfterSuccess", err)
	}
}

func TestExecNonzeroExitFailsAndCommitsNoRecord(t *testing.T) {
	state := t.TempDir()
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	outPath := filepath.Join(dir, "out.txt")
	spec := ExecSpec{Executable: tool, Outputs: []string{"out.txt"}, Dir: dir}

	runner := &scriptedRunner{exitCode: 1, stderr: []string{"boom"}}
	out := Init(Config{Isolated: true, StateDir: state, ProcessRunner: runner})
	err := runExecTask(t, out, "fails", spec)
	_ = out.Close()
	if !errors.Is(err, ErrExecNonzeroExit) {
		t.Fatalf("err = %v, want ErrExecNonzeroExit", err)
	}
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Fatal("nonzero exit must not fabricate the declared output")
	}

	// A following run with a succeeding runner must still spawn — no
	// success record was committed for the failed attempt.
	if err := os.WriteFile(outPath, []byte("built"), 0o644); err != nil {
		t.Fatal(err)
	}
	nextRunner := &scriptedRunner{exitCode: 0}
	next := Init(Config{Isolated: true, StateDir: state, ProcessRunner: nextRunner})
	t.Cleanup(func() { _ = next.Close() })
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
	spec := ExecSpec{Executable: tool, Outputs: []string{"out.txt"}, Dir: dir}
	runner := &scriptedRunner{exitCode: 0}
	out := Init(Config{Isolated: true, DryRun: true, StateDir: t.TempDir(), ProcessRunner: runner})
	t.Cleanup(func() { _ = out.Close() })

	if err := runExecTask(t, out, "dry", spec); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatal("dry-run Exec must never spawn")
	}
}

func TestExecCancellationKillsProcess(t *testing.T) {
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	spec := ExecSpec{Executable: tool}
	gate := make(chan struct{}) // never closed: only ctx cancellation may release Run
	runner := &scriptedRunner{exitCode: 0, gate: gate}
	out := Init(Config{Isolated: true, StateDir: t.TempDir(), ProcessRunner: runner})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	task := out.Task("cancel-me")
	task.Define(func(taskCtx context.Context) error {
		err := Exec(taskCtx, spec)
		done <- err
		return err
	})
	// Give the scheduler a moment to enter Define and reach the gated Run.
	time.Sleep(20 * time.Millisecond)
	cancel()
	_ = out.Run(ctx, func(context.Context) error { return nil })

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
		Executable: tool, Outputs: []string{"out.txt"}, Dir: dir,
		Basis: []fingerprint.Fingerprint{fingerprint.Value("input", 1)},
	}
	first := Init(Config{Isolated: true, StateDir: state, ProcessRunner: &scriptedRunner{exitCode: 0}})
	if err := runExecTask(t, first, "build", spec1); err != nil {
		t.Fatalf("first run: %v", err)
	}
	_ = first.Close()

	spec2 := spec1
	spec2.Basis = []fingerprint.Fingerprint{fingerprint.Value("input", 2)}
	runner := &scriptedRunner{exitCode: 0}
	second := Init(Config{Isolated: true, StateDir: state, ProcessRunner: runner})
	t.Cleanup(func() { _ = second.Close() })
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
	out := Init(Config{Isolated: true, StateDir: t.TempDir(), ProcessRunner: runner})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("narrated")
	var lastPhase string
	task.Define(func(ctx context.Context) error {
		err := Exec(ctx, ExecSpec{Executable: tool})
		lastPhase = task.Snapshot().Phase
		return err
	})
	_ = task.Wait()
	if lastPhase != "step two" {
		t.Fatalf("task phase = %q, want the last captured line", lastPhase)
	}
}

func TestExecCapturedSecretIsRedactedBeforeRetention(t *testing.T) {
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	runner := &scriptedRunner{exitCode: 1, stdout: []string{"token=super-secret-value"}}
	out := Init(Config{
		Isolated: true, StateDir: t.TempDir(), ProcessRunner: runner,
		Redactor: secretRedactor{secret: "super-secret-value"},
	})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("secret")
	var tail string
	task.Define(func(ctx context.Context) error {
		err := Exec(ctx, ExecSpec{Executable: tool})
		tail = task.EvidenceForTest().Text()
		return err
	})
	_ = task.Wait()
	if want := "super-secret-value"; strings.Contains(tail, want) {
		t.Fatalf("evidence tail leaked the secret: %q", tail)
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
// spec §11.6/§64's known-producer barrier itself, deterministically: once a
// producer has claimed a canonical Output (spec: claiming opens the gate
// immediately, before that operation's spawn even starts), a consumer's
// concurrent Basis fingerprint on that same path blocks until the producer
// settles, then observes its *final* written content — never an
// intermediate/missing state. Both Exec calls run concurrently from within
// one Task's Define (taskScope keys off ctx, not goroutine) so the test
// controls ordering directly instead of racing the scheduler's own Task
// start order — the scheduler is deliberately not exercised here; a real
// pipeline still declares After()/Sequence for real ordering, proven
// end-to-end in conformance/future/behavior/exec's §64 fixtures.
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
	router := routedRunner{byPath: map[string]ProcessRunner{
		normalizeTool: producerRunner,
		compileTool:   consumerRunner,
	}}

	out := Init(Config{Isolated: true, StateDir: t.TempDir(), ProcessRunner: router})
	t.Cleanup(func() { _ = out.Close() })

	consumerObservedSchema := make(chan string, 1)
	producerDone := make(chan struct{})
	task := out.Task("pipeline")
	task.Define(func(taskCtx context.Context) error {
		go func() {
			defer close(producerDone)
			_ = Exec(taskCtx, ExecSpec{Executable: normalizeTool, Dir: dir, Outputs: []string{"schema.json"}})
		}()

		// Wait for the producer's claim (spec: opened at claim time, before
		// its spawn) so the consumer's check is guaranteed to find the gate
		// already open — proving the barrier, not luck, does the blocking.
		canonPath := out.resolveWorkspacePath(schemaPath)
		for {
			out.mu.Lock()
			_, claimed := out.manifestClaims[canonPath]
			out.mu.Unlock()
			if claimed {
				break
			}
			time.Sleep(time.Millisecond)
		}

		go func() {
			time.Sleep(20 * time.Millisecond)
			close(gate)
		}()

		err := Exec(taskCtx, ExecSpec{
			Executable: compileTool, Dir: dir,
			Basis:   []fingerprint.Fingerprint{fingerprint.FSPath(schemaPath)},
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
	_ = task.Wait()

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
	out := Init(Config{Isolated: true, StateDir: t.TempDir(), ProcessRunner: &scriptedRunner{exitCode: 0}})
	t.Cleanup(func() { _ = out.Close() })

	first := out.Task("first")
	first.Define(func(ctx context.Context) error {
		return Exec(ctx, ExecSpec{Executable: tool, Dir: dir, Outputs: []string{"shared.txt"}})
	})
	_ = first.Wait()

	var secondErr error
	second := out.Task("second")
	second.Define(func(ctx context.Context) error {
		secondErr = Exec(ctx, ExecSpec{Executable: tool, Dir: dir, Outputs: []string{"shared.txt"}})
		return secondErr
	})
	_ = second.Wait()

	if !errors.Is(secondErr, ErrFileConflictingProducer) {
		t.Fatalf("second claim err = %v, want ErrFileConflictingProducer", secondErr)
	}
}
