//go:build evopending

// Pending: ZYS-1204 Attempts. Red today because the package does not
// compile. GUESSED SIGNATURES (the contract names none):
//
//	(*GroupHandle).Attempts(name string) *AttemptsHandle
//	(*AttemptsHandle).Max(n int) *AttemptsHandle
//	(*AttemptsHandle).Define(func(ctx context.Context, prior []evo.AttemptSnapshot) error)
//	evo.ProblemCodeVerificationUnproven (string code on the failed Task)
package attempts_test

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func newOutput(t *testing.T) *evo.Output {
	t.Helper()
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	return out
}

func TestC31_021_FailedAttemptDoesNotFailParentAndRetriesUntilVerifyPasses(t *testing.T) {
	out := newOutput(t)
	var tries atomic.Int32
	parent := out.Group("converge")
	attempts := parent.Attempts("fix formatting").Max(3)
	attempts.Define(func(ctx context.Context, prior []evo.AttemptSnapshot) error {
		if int(tries.Add(1)) != len(prior)+1 {
			return errors.New("prior attempts not passed through")
		}
		return nil
	})
	attempts.Verify(func(context.Context) (bool, error) { return tries.Load() >= 2, nil })
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v (a failed attempt must not fail the parent)", err)
	}
	if tries.Load() != 2 {
		t.Fatalf("attempts = %d, want 2 (stop once Verify passes)", tries.Load())
	}
}

func TestC31_021_FailedAfterCallbackVerifyIsVerificationUnproven(t *testing.T) {
	out := newOutput(t)
	task := out.Task("deploy service").Verify(func(context.Context) (bool, error) {
		return false, errors.New("cannot observe service")
	})
	task.Define(func(context.Context) error { return nil })
	_ = out.Finish()
	snap := task.Snapshot()
	if snap.State != evo.Failed {
		t.Fatalf("state = %v, want Failed", snap.State)
	}
	for _, p := range snap.Problems {
		if p.Code == evo.ProblemCodeVerificationUnproven {
			return
		}
	}
	t.Fatalf("no Problem with code %q in %+v", evo.ProblemCodeVerificationUnproven, snap.Problems)
}
