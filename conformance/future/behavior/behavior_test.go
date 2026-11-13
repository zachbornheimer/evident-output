//go:build v06acceptance

// Package futurebehavior_test holds increment-1's behavioral fixtures for
// §9.1 Verify — already implemented in this worktree. It compiles and
// passes under `go test -tags v06acceptance ./conformance/future/behavior`;
// increment 2+ fixtures (File/Exec/dry-run/cancellation/manifest re-entry)
// live in the sibling ./pending package instead, which stays red-by-compile
// until those increments land.
package futurebehavior_test

import (
	"context"
	"errors"
	"os"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func isolated(t *testing.T) *evo.Output {
	t.Helper()
	out := evo.Init(evo.Config{Isolated: true, Stdout: os.Stdout, Stderr: os.Stderr})
	t.Cleanup(func() { _ = out.Close() })
	return out
}

// TestV06VerifyTrueSkipsDefine proves a true pre-Define Verify both skips the
// callback AND lets the Task settle successfully.
func TestV06VerifyTrueSkipsDefine(t *testing.T) {
	out := isolated(t)
	called := false
	out.Run(context.Background(), func(ctx context.Context) error {
		task := out.Task("verified")
		task.Verify(func(context.Context) (bool, error) { return true, nil })
		task.Define(func(context.Context) error { called = true; return nil })
		return nil
	})
	if called {
		t.Fatal("Verify true must skip Define")
	}
	if out.Conclusion().ExitCode != evo.ExitOK {
		t.Fatalf("Verify true must still settle the run successfully, got exit=%d", out.Conclusion().ExitCode)
	}
}

// TestV06VerifyFalseEntersDefine's Verify reports the desired state as it
// really is at each check: not yet satisfied before Define runs, satisfied
// after — Define's own callback is what's expected to make that true, the
// same shape a filesystem/API observation would have. A stateless
// always-false Verify would (correctly, per §9.1) fail the post-Define
// check too; this proves the true "false enters Define, Define's work
// satisfies it" path instead.
func TestV06VerifyFalseEntersDefine(t *testing.T) {
	out := isolated(t)
	called := false
	checks := 0
	out.Run(context.Background(), func(ctx context.Context) error {
		task := out.Task("unverified")
		task.Verify(func(context.Context) (bool, error) {
			checks++
			return checks > 1, nil // false pre-Define, true post-Define
		})
		task.Define(func(context.Context) error { called = true; return nil })
		return nil
	})
	if !called {
		t.Fatal("Verify false must enter Define")
	}
	if out.Conclusion().ExitCode != evo.ExitOK {
		t.Fatalf("Verify false followed by a succeeding Define must settle successfully, got exit=%d", out.Conclusion().ExitCode)
	}
}

func TestV06VerifyErrorPreventsMutation(t *testing.T) {
	out := isolated(t)
	called := false
	out.Run(context.Background(), func(ctx context.Context) error {
		task := out.Task("observation-error")
		task.Verify(func(context.Context) (bool, error) { return false, errors.New("unavailable") })
		task.Define(func(context.Context) error { called = true; return nil })
		return nil
	})
	if called {
		t.Fatal("Verify error must prevent Define mutation")
	}
	if out.Conclusion().ExitCode == evo.ExitOK {
		t.Fatal("pre-Define Verify observation error must fail the task, not silently proceed")
	}
}

func TestV06PostVerifyFalseFails(t *testing.T) {
	out := isolated(t)
	checks := 0
	out.Run(context.Background(), func(ctx context.Context) error {
		task := out.Task("post-false")
		task.Verify(func(context.Context) (bool, error) { checks++; return false, nil })
		task.Define(func(context.Context) error { return nil })
		return nil
	})
	if checks != 2 {
		t.Fatalf("Verify checks = %d, want pre and post checks", checks)
	}
	if out.Conclusion().ExitCode == evo.ExitOK {
		t.Fatal("post-Define false must fail the task")
	}
}

// TestV06PostVerifyErrorFails proves a post-Define Verify OBSERVATION ERROR
// fails the task distinctly from a post-Define false: §9.1 requires both a
// postcondition-not-satisfied result and an observation failure to fail the
// task, and both must be reachable without collapsing into one code path.
func TestV06PostVerifyErrorFails(t *testing.T) {
	out := isolated(t)
	checks := 0
	out.Run(context.Background(), func(ctx context.Context) error {
		task := out.Task("post-error")
		task.Verify(func(context.Context) (bool, error) {
			checks++
			if checks == 1 {
				return false, nil // pre-Define: not yet satisfied, enter Define
			}
			return false, errors.New("observation unavailable") // post-Define: error
		})
		task.Define(func(context.Context) error { return nil })
		return nil
	})
	if checks != 2 {
		t.Fatalf("Verify checks = %d, want pre and post checks", checks)
	}
	if out.Conclusion().ExitCode == evo.ExitOK {
		t.Fatal("post-Define Verify observation error must fail the task")
	}
}
