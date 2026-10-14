package change

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// shellTool is an executable that exists on every platform the tests run on.
const shellTool = "/bin/sh"

func TestExecRequiresAnExecutable(t *testing.T) {
	_, ctx := scopedRun(t, newFakeHost(t.TempDir()))

	if _, err := Exec(ctx, ExecSpec{}); !errors.Is(err, ErrExecSpecMissingExecutable) {
		t.Fatalf("Exec without an executable = %v, want ErrExecSpecMissingExecutable", err)
	}
}

func TestExecWithAnUnresolvableNameReturnsErrExecExecutableNotFound(t *testing.T) {
	host := newFakeHost(t.TempDir())
	_, ctx := scopedRun(t, host)

	_, err := Exec(ctx, ExecSpec{Executable: "evo-no-such-tool-anywhere"})

	if !errors.Is(err, ErrExecExecutableNotFound) || len(host.spawned) != 0 {
		t.Fatalf("err = %v, spawned = %v; want ErrExecExecutableNotFound and no spawn", err, host.spawned)
	}
}

func TestExecSpawnsAResolvedCommandVerifiesOutputsAndRecordsOnce(t *testing.T) {
	dir := t.TempDir()
	host := newFakeHost(t.TempDir())
	host.spawn = func(cmd SpawnCommand) (ExecResult, error) {
		if err := os.WriteFile(filepath.Join(cmd.Dir, "made.txt"), []byte("made"), 0o600); err != nil {
			t.Error(err)
		}
		return ExecResult{Ran: true, Stdout: "hello"}, nil
	}
	_, ctx := scopedRun(t, host)

	result, err := Exec(ctx, ExecSpec{Executable: shellTool, Args: []string{"-c", "true"}, Dir: dir, Outputs: []string{"made.txt"}, Env: map[string]string{"EVO_CHANGE_TEST": "1"}})

	if err != nil || !result.Ran || result.Stdout != "hello" {
		t.Fatalf("Exec = (%+v, %v), want the spawn's own result", result, err)
	}
	if len(host.spawned) != 1 || host.spawned[0].Path != shellTool || host.spawned[0].Dir != dir {
		t.Fatalf("spawned %+v, want one /bin/sh in %s", host.spawned, dir)
	}
	if !slices.Contains(host.spawned[0].Env, "EVO_CHANGE_TEST=1") {
		t.Errorf("child env lacks the spec's own entry: %v", host.spawned[0].Env)
	}
	if got := host.recorded(); len(got) != 1 || got[0].Verb() != "run" {
		t.Errorf("recorded %v, want one run entry", got)
	}
	if len(host.appended) != 1 || len(host.appended[0].Outputs) != 1 {
		t.Errorf("appended %+v, want one operation record with the one output", host.appended)
	}
	want := []ObservationKind{BasisFingerprinted, OperationStarted, ResourceObserved, OperationFinished}
	if got := host.observations(); !slices.Equal(got, want) {
		t.Errorf("observed %v, want %v", got, want)
	}
}

func TestExecNonzeroExitFailsAndStillReturnsTheAttempt(t *testing.T) {
	host := newFakeHost(t.TempDir())
	host.spawn = func(SpawnCommand) (ExecResult, error) { return ExecResult{Ran: true, ExitCode: 3, Stderr: "bad"}, nil }
	_, ctx := scopedRun(t, host)

	result, err := Exec(ctx, ExecSpec{Executable: shellTool})

	if !errors.Is(err, ErrExecNonzeroExit) || result.ExitCode != 3 || result.Stderr != "bad" {
		t.Fatalf("Exec = (%+v, %v), want the attempt and ErrExecNonzeroExit", result, err)
	}
	if len(host.recorded()) != 0 || len(host.appended) != 0 {
		t.Error("a failed Exec recorded an effect or an operation")
	}
}

func TestExecWithAMissingDeclaredOutputFailsVerification(t *testing.T) {
	host := newFakeHost(t.TempDir())
	_, ctx := scopedRun(t, host)

	result, err := Exec(ctx, ExecSpec{Executable: shellTool, Dir: t.TempDir(), Outputs: []string{"never-made.txt"}})

	if !errors.Is(err, ErrExecOutputMissingAfterSuccess) || !result.Ran {
		t.Fatalf("Exec = (%+v, %v), want the attempt and ErrExecOutputMissingAfterSuccess", result, err)
	}
}

func TestExecSpawnFailureReturnsAZeroResult(t *testing.T) {
	boom := errors.New("cannot start")
	host := newFakeHost(t.TempDir())
	host.spawn = func(SpawnCommand) (ExecResult, error) { return ExecResult{}, boom }
	_, ctx := scopedRun(t, host)

	result, err := Exec(ctx, ExecSpec{Executable: shellTool})

	if !errors.Is(err, boom) || result != (ExecResult{}) {
		t.Fatalf("Exec = (%+v, %v), want a zero result and the spawn error", result, err)
	}
}

func TestExecWithoutOutputsRecordsNoLedgerEntry(t *testing.T) {
	for _, applies := range []bool{true, false} {
		host := newFakeHost(t.TempDir())
		host.applies = applies
		host.spawn = func(SpawnCommand) (ExecResult, error) { return ExecResult{Ran: true}, nil }
		_, ctx := scopedRun(t, host)

		if _, err := Exec(ctx, ExecSpec{Executable: shellTool}); err != nil {
			t.Fatalf("applies=%v: Exec = %v", applies, err)
		}

		if got := host.recorded(); len(got) != 0 {
			t.Errorf("applies=%v: recorded %v, want no ledger entry for an Exec that declared no Outputs", applies, got)
		}
		for _, observation := range host.observed {
			if observation.Kind == OperationFinished && observation.Changed {
				t.Errorf("applies=%v: the finished Exec was observed as a change", applies)
			}
		}
	}
}

func TestExecDryRunPlansTheRunWithoutSpawning(t *testing.T) {
	host := newFakeHost(t.TempDir())
	host.applies = false
	_, ctx := scopedRun(t, host)

	result, err := Exec(ctx, ExecSpec{Executable: shellTool, Outputs: []string{"made.txt"}})

	if err != nil || result.Ran || len(host.spawned) != 0 {
		t.Fatalf("planned Exec = (%+v, %v), spawned %v; want Ran false and no spawn", result, err, host.spawned)
	}
	if got := host.recorded(); len(got) != 1 || host.planned != 1 {
		t.Errorf("recorded %d entries, planned %d; want 1 and 1", len(got), host.planned)
	}
}

func TestExecClaimsItsOutputsAgainstOtherTasks(t *testing.T) {
	run, ctx := scopedRun(t, newFakeHost(t.TempDir()))
	dir := t.TempDir()
	if err := run.claimOutput("someone-else", filepath.Join(dir, "made.txt")); err != nil {
		t.Fatal(err)
	}

	_, err := Exec(ctx, ExecSpec{Executable: shellTool, Dir: dir, Outputs: []string{"made.txt"}})

	if !errors.Is(err, ErrFileConflictingProducer) {
		t.Fatalf("Exec over another Task's output = %v, want ErrFileConflictingProducer", err)
	}
}

func TestMergedExecEnvOverridesInheritedNamesAndStaysSorted(t *testing.T) {
	t.Setenv("EVO_CHANGE_INHERITED", "outer")

	env := mergedExecEnv(map[string]string{"EVO_CHANGE_INHERITED": "inner", "EVO_CHANGE_ADDED": "new"})

	if !slices.Contains(env, "EVO_CHANGE_INHERITED=inner") || slices.Contains(env, "EVO_CHANGE_INHERITED=outer") || !slices.Contains(env, "EVO_CHANGE_ADDED=new") {
		t.Errorf("overrides not applied: %v", env)
	}
	if !slices.IsSorted(env) {
		t.Error("a merged environment must be sorted so the child's order is deterministic")
	}
	if none := mergedExecEnv(nil); len(none) == 0 || strings.Contains(strings.Join(none, "\n"), "EVO_CHANGE_ADDED") {
		t.Errorf("no overrides must return the process environment unchanged")
	}
}
