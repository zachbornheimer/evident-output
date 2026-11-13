package evo_test

import (
	"context"
	"errors"
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestTaskSnapshot_ResolutionNoWorkByDefault proves §29/§30's default: a
// Task explicitly resolved successfully without ever reaching Define
// carries ResolutionNoWork and an unevaluated (zero-value) Evidence.
func TestTaskSnapshot_ResolutionNoWorkByDefault(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("no-define")
	task.Done()
	snap := task.Snapshot()

	if snap.Resolution != evo.ResolutionNoWork {
		t.Fatalf("Resolution = %q, want ResolutionNoWork", snap.Resolution)
	}
	if snap.Evidence.Before.Evaluated || snap.Evidence.After.Evaluated {
		t.Fatalf("Evidence = %+v, want both phases unevaluated", snap.Evidence)
	}
}

// TestTaskSnapshot_ResolutionExecutedWithoutVerify proves §29/§30: a Task
// whose Define callback ran and returned successfully, with no Verify
// registered, carries ResolutionExecuted and unevaluated Evidence — a
// Verify-less Define reaches the same success it always has, just now with
// a Resolution recorded.
func TestTaskSnapshot_ResolutionExecutedWithoutVerify(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("plain-define")
	task.Define(func(context.Context) error { return nil })
	_ = task.Wait()
	snap := task.Snapshot()

	if snap.Resolution != evo.ResolutionExecuted {
		t.Fatalf("Resolution = %q, want ResolutionExecuted", snap.Resolution)
	}
	if snap.Evidence.Before.Evaluated || snap.Evidence.After.Evaluated {
		t.Fatalf("Evidence = %+v, want both phases unevaluated (no Verify registered)", snap.Evidence)
	}
}

// TestTaskSnapshot_ResolutionAlreadySatisfiedPreservesBeforeEvidence proves
// §30's before-satisfied evidence phase: a pre-Define Verify that returns
// true records Evidence.Before{Evaluated:true, Satisfied:true, Source:
// "verify"} and resolution ResolutionAlreadySatisfied, with After left
// unevaluated since Define never ran.
func TestTaskSnapshot_ResolutionAlreadySatisfiedPreservesBeforeEvidence(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("already-satisfied")
	task.Verify(func(context.Context) (bool, error) { return true, nil })
	task.Define(func(context.Context) error { return nil })
	_ = task.Wait()
	snap := task.Snapshot()

	if snap.Resolution != evo.ResolutionAlreadySatisfied {
		t.Fatalf("Resolution = %q, want ResolutionAlreadySatisfied", snap.Resolution)
	}
	before := snap.Evidence.Before
	if !before.Evaluated || !before.Satisfied || before.Source != "verify" {
		t.Fatalf("Evidence.Before = %+v, want {Evaluated:true Satisfied:true Source:verify}", before)
	}
	if snap.Evidence.After.Evaluated {
		t.Fatalf("Evidence.After = %+v, want unevaluated (Define never ran)", snap.Evidence.After)
	}
}

// TestTaskSnapshot_BeforeFalseAfterTruePreservesBothPhases proves §30's
// central non-abusable-boolean claim: "A Task can therefore be
// resolution=executed while evidence.after.satisfied=true ... Preserve the
// before-false/after-true transition rather than overwriting it with one
// final boolean." Before is recorded false, then After true, and both
// survive on the snapshot simultaneously.
func TestTaskSnapshot_BeforeFalseAfterTruePreservesBothPhases(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("before-false-after-true")
	checks := 0
	task.Verify(func(context.Context) (bool, error) {
		checks++
		return checks > 1, nil
	})
	task.Define(func(context.Context) error { return nil })
	_ = task.Wait()
	snap := task.Snapshot()

	if snap.Resolution != evo.ResolutionExecuted {
		t.Fatalf("Resolution = %q, want ResolutionExecuted", snap.Resolution)
	}
	before, after := snap.Evidence.Before, snap.Evidence.After
	if !before.Evaluated || before.Satisfied {
		t.Fatalf("Evidence.Before = %+v, want {Evaluated:true Satisfied:false}", before)
	}
	if !after.Evaluated || !after.Satisfied {
		t.Fatalf("Evidence.After = %+v, want {Evaluated:true Satisfied:true}", after)
	}
}

// TestResult_ExitCodeMapping pins §31's recommended mapping (OK 0, Blocked
// 1, Failed 2, Cancelled 130) through the real Result.ExitCode() accessor —
// a printed conclusion and the exit code must never disagree, and this is
// the one method both read from.
func TestResult_ExitCodeMapping(t *testing.T) {
	cases := []struct {
		name string
		run  func(out *evo.Output) evo.RunFunc
		want int
	}{
		{"ok", func(out *evo.Output) evo.RunFunc {
			return func(ctx context.Context) error { return nil }
		}, evo.ExitOK},
		{"blocked", func(out *evo.Output) evo.RunFunc {
			return func(ctx context.Context) error {
				out.Task("gate").Block("no")
				return nil
			}
		}, evo.ExitBlocked},
		{"failed", func(out *evo.Output) evo.RunFunc {
			return func(ctx context.Context) error { return errors.New("boom") }
		}, evo.ExitFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
			t.Cleanup(func() { _ = out.Close() })
			result := out.Run(context.Background(), c.run(out))
			if result.ExitCode() != c.want {
				t.Fatalf("ExitCode() = %d, want %d", result.ExitCode(), c.want)
			}
			if result.Conclusion.ExitCode != result.ExitCode() {
				t.Fatalf("Conclusion.ExitCode = %d disagrees with Result.ExitCode() = %d", result.Conclusion.ExitCode, result.ExitCode())
			}
		})
	}
}
