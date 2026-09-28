package engine

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestGroupHandle_Wait_AllSucceed pins ZYS-849's baseline: a Group can be
// awaited directly after every child is declared, and a fully successful
// Group's Wait returns nil without the caller ever snapshotting/counting
// children itself.
func TestGroupHandle_Wait_AllSucceed(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{
		Isolated: true, Plain: true, Color: ColorNever,
		MaxConcurrency: 4, Stdout: &buf, Stderr: io.Discard,
	})

	res := out.Run(context.Background(), func(ctx context.Context) error {
		jobs := out.Group("check")
		for _, name := range []string{"a", "b", "c"} {
			jobs.Task(name).Define(func(ctx context.Context) error { return nil })
		}
		return jobs.Wait()
	})
	if res.ExitCode() != ExitOK {
		t.Fatalf("exit %d, want ExitOK; output:\n%s", res.ExitCode(), buf.String())
	}
}

// TestGroupHandle_Wait_DoesNotSerializeSiblings proves Group.Wait does not
// itself force one-at-a-time execution: every child must reach its blocking
// gate concurrently before any of them is released, which is only possible
// if the scheduler ran them in parallel rather than Wait looping Task.Wait
// one child at a time before the next child's callback even starts.
func TestGroupHandle_Wait_DoesNotSerializeSiblings(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{
		Isolated: true, Plain: true, Color: ColorNever,
		MaxConcurrency: 3, Stdout: &buf, Stderr: io.Discard,
	})

	const n = 3
	arrived := make(chan struct{}, n)
	release := make(chan struct{})
	runCtx := context.Background()

	results := make(chan Result, 1)
	go func() {
		results <- out.Run(runCtx, func(ctx context.Context) error {
			jobs := out.Group("parallel")
			for _, name := range []string{"x", "y", "z"} {
				jobs.Task(name).Define(func(ctx context.Context) error {
					arrived <- struct{}{}
					<-release
					return nil
				})
			}
			return jobs.Wait()
		})
	}()

	budget := time.After(2 * time.Second)
	for i := range n {
		select {
		case <-arrived:
		case <-budget:
			t.Fatalf("only %d/%d siblings started concurrently before timeout", i, n)
		}
	}
	close(release)

	var res Result
	select {
	case res = <-results:
	case <-budget:
		t.Fatal("Group.Wait did not return after all siblings were released")
	}
	if res.ExitCode() != ExitOK {
		t.Fatalf("exit %d, want ExitOK; output:\n%s", res.ExitCode(), buf.String())
	}
}

// TestGroupHandle_Wait_FailedChildIsDeterministic pins the container-failure
// contract: a failed child produces a deterministic, non-nil Wait error, and
// the caller never had to Snapshot/count children to discover it.
func TestGroupHandle_Wait_FailedChildIsDeterministic(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{
		Isolated: true, Plain: true, Color: ColorNever,
		MaxConcurrency: 4, Stdout: &buf, Stderr: io.Discard,
	})

	boom := errors.New("boom")
	res := out.Run(context.Background(), func(ctx context.Context) error {
		jobs := out.Group("check")
		jobs.Task("a").Define(func(ctx context.Context) error { return nil })
		jobs.Task("b").Define(func(ctx context.Context) error { return boom })
		jobs.Task("c").Define(func(ctx context.Context) error { return nil })
		err := jobs.Wait()
		if err == nil {
			t.Fatal("jobs.Wait() = nil, want a deterministic failure")
		}
		if !errors.Is(err, boom) {
			t.Fatalf("jobs.Wait() = %v, want errors.Is(err, boom)", err)
		}
		return err
	})
	if res.ExitCode() == ExitOK {
		t.Fatalf("exit ExitOK, want a failing exit; output:\n%s", buf.String())
	}
}

// TestSequenceHandle_Wait_FailedStepOmitsNotStartedFollowers pins the
// NotStarted-suppression rule from the ZYS-849 Decisions: a failed
// predecessor's ErrNotStarted-caused followers are not duplicated into the
// joined error, but the predecessor's own failure is.
func TestSequenceHandle_Wait_FailedStepOmitsNotStartedFollowers(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{
		Isolated: true, Plain: true, Color: ColorNever,
		MaxConcurrency: 1, Stdout: &buf, Stderr: io.Discard,
	})

	boom := errors.New("scan failed")
	res := out.Run(context.Background(), func(ctx context.Context) error {
		seq := out.Sequence("python")
		seq.Task("scan").Define(func(ctx context.Context) error { return boom })
		seq.Task("venv").Define(func(ctx context.Context) error { return nil })
		seq.Task("install").Define(func(ctx context.Context) error { return nil })
		err := seq.Wait()
		if err == nil {
			t.Fatal("seq.Wait() = nil, want the scan failure")
		}
		if !errors.Is(err, boom) {
			t.Fatalf("seq.Wait() = %v, want errors.Is(err, boom)", err)
		}
		if errors.Is(err, ErrNotStarted) {
			t.Fatalf("seq.Wait() = %v, must not surface ErrNotStarted for a predecessor-caused follower", err)
		}
		if got := strings.Count(err.Error(), "scan failed"); got != 1 {
			t.Fatalf("joined error mentions the predecessor failure %d times, want 1: %v", got, err)
		}
		return err
	})
	if res.ExitCode() == ExitOK {
		t.Fatalf("exit ExitOK, want a failing exit; output:\n%s", buf.String())
	}
}

// TestGroupHandle_Wait_BlockedChildSurfacesFailure pins the "blocked ...
// descendants have defined semantics" acceptance line: a child that resolves
// Blocked (via the standard `return task.Blockf(...)` Define idiom) makes
// the container Wait fail deterministically, exactly like a Failed child.
func TestGroupHandle_Wait_BlockedChildSurfacesFailure(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{
		Isolated: true, Plain: true, Color: ColorNever,
		MaxConcurrency: 4, Stdout: &buf, Stderr: io.Discard,
	})

	res := out.Run(context.Background(), func(ctx context.Context) error {
		jobs := out.Group("check")
		jobs.Task("a").Define(func(ctx context.Context) error { return nil })
		blocked := jobs.Task("b")
		blocked.Define(func(ctx context.Context) error {
			blocked.Block("needs manual review", Detail("ambiguous"))
			return nil
		})
		err := jobs.Wait()
		if err == nil {
			t.Fatal("jobs.Wait() = nil, want the Blocked child's failure")
		}
		if got := blocked.Snapshot().State; got != Blocked {
			t.Fatalf("child state = %v, want Blocked", got)
		}
		// The header agrees with what Tasks After the Group see: it
		// finished, and it did not succeed.
		if got := jobs.Snapshot().State; got != Blocked {
			t.Errorf("group state = %v, want Blocked", got)
		}
		nested := out.Group("outer")
		inner := nested.Group("inner")
		innerBlocked := inner.Task("c")
		innerBlocked.Define(func(ctx context.Context) error { innerBlocked.Block("held"); return nil })
		_ = nested.Wait()
		if got := nested.Snapshot().State; got != Blocked {
			t.Errorf("outer group state = %v, want Blocked from its nested Group", got)
		}
		return err
	})
	if res.ExitCode() == ExitOK {
		t.Fatalf("exit ExitOK, want a failing exit; output:\n%s", buf.String())
	}
}

// TestGroupHandle_Wait_CancelledChildIsVisible pins the "cancelled ...
// descendants have defined semantics" acceptance line: a child resolved
// Cancelled makes the container Wait return a non-nil, errors.Is-compatible
// cancellation error, without the caller snapshotting/counting children.
func TestGroupHandle_Wait_CancelledChildIsVisible(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{
		Isolated: true, Plain: true, Color: ColorNever,
		MaxConcurrency: 4, Stdout: &buf, Stderr: io.Discard,
	})

	res := out.Run(context.Background(), func(ctx context.Context) error {
		jobs := out.Group("check")
		jobs.Task("a").Define(func(ctx context.Context) error { return nil })
		jobs.Task("b").Cancel("superseded")
		err := jobs.Wait()
		if err == nil {
			t.Fatal("jobs.Wait() = nil, want the Cancelled child's outcome")
		}
		if !errors.Is(err, errWaitCancelled) {
			t.Fatalf("jobs.Wait() = %v, want errors.Is(err, errWaitCancelled)", err)
		}
		return err
	})
	if res.ExitCode() == ExitOK {
		t.Fatalf("exit ExitOK, want a failing exit; output:\n%s", buf.String())
	}
}

// TestSequenceHandle_Wait_PreservesOrdering pins Sequence.Wait's second
// contract half: ordering is still enforced (venv never starts before scan
// completes), proven by an atomic step counter each callback checks.
func TestSequenceHandle_Wait_PreservesOrdering(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{
		Isolated: true, Plain: true, Color: ColorNever,
		MaxConcurrency: 4, Stdout: &buf, Stderr: io.Discard,
	})

	var step int32
	res := out.Run(context.Background(), func(ctx context.Context) error {
		seq := out.Sequence("ordered")
		seq.Task("scan").Define(func(ctx context.Context) error {
			if !atomic.CompareAndSwapInt32(&step, 0, 1) {
				t.Error("scan did not run first")
			}
			return nil
		})
		seq.Task("venv").Define(func(ctx context.Context) error {
			if !atomic.CompareAndSwapInt32(&step, 1, 2) {
				t.Error("venv did not run second")
			}
			return nil
		})
		seq.Task("install").Define(func(ctx context.Context) error {
			if !atomic.CompareAndSwapInt32(&step, 2, 3) {
				t.Error("install did not run third")
			}
			return nil
		})
		return seq.Wait()
	})
	if res.ExitCode() != ExitOK {
		t.Fatalf("exit %d, want ExitOK; output:\n%s", res.ExitCode(), buf.String())
	}
}

// TestTaskHandle_Define_ReturnsHandleForChaining pins the Decisions' Define
// sugar: Define returns the same *TaskHandle so `task.Define(fn).Wait()`
// composes without a separate local variable, and this must not change
// Define's asynchronous scheduler semantics (the callback still runs on the
// scheduler, not inline).
func TestTaskHandle_Define_ReturnsHandleForChaining(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{
		Isolated: true, Plain: true, Color: ColorNever,
		MaxConcurrency: 4, Stdout: &buf, Stderr: io.Discard,
	})

	res := out.Run(context.Background(), func(ctx context.Context) error {
		task := out.Task("solo")
		return task.Define(func(ctx context.Context) error { return nil }).Wait()
	})
	if res.ExitCode() != ExitOK {
		t.Fatalf("exit %d, want ExitOK; output:\n%s", res.ExitCode(), buf.String())
	}
}

// TestGroupHandle_Wait_ExternalPredecessorFailure pins that Wait never
// reports success for a container none of whose children ran: when every
// child is NotStarted because a predecessor OUTSIDE the container failed,
// that cause is not in the container's own join, so the NotStarted outcome
// must surface rather than be dropped as "already represented".
func TestGroupHandle_Wait_ExternalPredecessorFailure(t *testing.T) {
	out := Init(Config{
		Isolated: true, Plain: true, Color: ColorNever,
		MaxConcurrency: 2, Stdout: io.Discard, Stderr: io.Discard,
	})
	var ran atomic.Bool
	var waitErr error
	out.Run(context.Background(), func(ctx context.Context) error {
		pre := out.Task("prepare")
		pre.Define(func(ctx context.Context) error { return errors.New("boom") })
		jobs := out.Group("jobs")
		jobs.Task("a").After(pre).Define(func(ctx context.Context) error {
			ran.Store(true)
			return nil
		})
		waitErr = jobs.Wait()
		return nil
	})
	if ran.Load() {
		t.Fatal("child ran despite its failed predecessor")
	}
	if !errors.Is(waitErr, ErrNotStarted) {
		t.Fatalf("Group.Wait = %v, want ErrNotStarted when no child ran", waitErr)
	}
}

// TestSequenceHandle_Wait_ExternalPredecessorFailure is the Sequence
// counterpart of TestGroupHandle_Wait_ExternalPredecessorFailure.
func TestSequenceHandle_Wait_ExternalPredecessorFailure(t *testing.T) {
	out := Init(Config{
		Isolated: true, Plain: true, Color: ColorNever,
		MaxConcurrency: 2, Stdout: io.Discard, Stderr: io.Discard,
	})
	var waitErr error
	out.Run(context.Background(), func(ctx context.Context) error {
		pre := out.Task("prepare")
		pre.Define(func(ctx context.Context) error { return errors.New("boom") })
		seq := out.Sequence("steps")
		seq.Task("first").After(pre).Define(func(ctx context.Context) error { return nil })
		seq.Task("second").Define(func(ctx context.Context) error { return nil })
		waitErr = seq.Wait()
		return nil
	})
	if !errors.Is(waitErr, ErrNotStarted) {
		t.Fatalf("Sequence.Wait = %v, want ErrNotStarted when no step ran", waitErr)
	}
}
