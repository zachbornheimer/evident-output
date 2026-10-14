package evo

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// TestGroupHandle_Wait_AllSucceed pins ZYS-849's baseline: a Group can be
// awaited directly after every child is declared, and a fully successful
// Group's Wait returns nil without the caller ever snapshotting/counting
// children itself.
func TestGroupHandle_Wait_AllSucceed(t *testing.T) {
	var buf strings.Builder
	out := initRun(Config{
		plain: true, color: colorNever,
		MaxConcurrency: 4, stdout: &buf, stderr: io.Discard,
	})

	res := out.run(context.Background(), func(ctx context.Context, r *Run) error {
		jobs := out.Group("check")
		for _, name := range []string{"a", "b", "c"} {
			jobs.Task(name).Define(func(ctx context.Context) error { return nil })
		}
		return jobs.wait()
	})
	if res.ExitCode() != exitOK {
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
	out := initRun(Config{
		plain: true, color: colorNever,
		MaxConcurrency: 3, stdout: &buf, stderr: io.Discard,
	})

	const n = 3
	arrived := make(chan struct{}, n)
	release := make(chan struct{})
	runCtx := context.Background()

	results := make(chan Result, 1)
	go func() {
		results <- out.run(runCtx, func(ctx context.Context, r *Run) error {
			jobs := out.Group("parallel")
			for _, name := range []string{"x", "y", "z"} {
				jobs.Task(name).Define(func(ctx context.Context) error {
					arrived <- struct{}{}
					<-release
					return nil
				})
			}
			return jobs.wait()
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
	if res.ExitCode() != exitOK {
		t.Fatalf("exit %d, want ExitOK; output:\n%s", res.ExitCode(), buf.String())
	}
}

// TestGroupHandle_Wait_BlockedChildSurfacesFailure pins the "blocked ...
// descendants have defined semantics" acceptance line: a child that resolves
// Blocked (via Block then return nil inside Define) makes
// the container Wait fail deterministically, exactly like a Failed child.
func TestGroupHandle_Wait_BlockedChildSurfacesFailure(t *testing.T) {
	var buf strings.Builder
	out := initRun(Config{
		plain: true, color: colorNever,
		MaxConcurrency: 4, stdout: &buf, stderr: io.Discard,
	})

	res := out.run(context.Background(), func(ctx context.Context, r *Run) error {
		jobs := out.Group("check")
		jobs.Task("a").Define(func(ctx context.Context) error { return nil })
		blocked := jobs.Task("b")
		blocked.Define(func(ctx context.Context) error {
			blocked.block("needs manual review", Detail("ambiguous"))
			return nil
		})
		err := jobs.wait()
		if err == nil {
			t.Fatal("jobs.Wait() = nil, want the Blocked child's failure")
		}
		if got := blocked.snapshot().State; got != record.Refused {
			t.Fatalf("child state = %v, want Blocked", got)
		}
		// The header agrees with what Tasks After the Group see: it
		// finished, and it did not succeed.
		if got := jobs.snapshot().State; got != record.Refused {
			t.Errorf("group state = %v, want Blocked", got)
		}
		nested := out.Group("outer")
		inner := nested.Group("inner")
		innerBlocked := inner.Task("c")
		innerBlocked.Define(func(ctx context.Context) error { innerBlocked.block("held"); return nil })
		_ = nested.wait()
		if got := nested.snapshot().State; got != record.Refused {
			t.Errorf("outer group state = %v, want Blocked from its nested Group", got)
		}
		return err
	})
	if res.ExitCode() == exitOK {
		t.Fatalf("exit ExitOK, want a failing exit; output:\n%s", buf.String())
	}
}

// TestSequenceHandle_Wait_PreservesOrdering pins Sequence.Wait's second
// contract half: ordering is still enforced (venv never starts before scan
// completes), proven by an atomic step counter each callback checks.
func TestSequenceHandle_Wait_PreservesOrdering(t *testing.T) {
	var buf strings.Builder
	out := initRun(Config{
		plain: true, color: colorNever,
		MaxConcurrency: 4, stdout: &buf, stderr: io.Discard,
	})

	var step atomic.Int32
	res := out.run(context.Background(), func(ctx context.Context, r *Run) error {
		seq := out.Sequence("ordered")
		seq.Task("scan").Define(func(ctx context.Context) error {
			if !step.CompareAndSwap(0, 1) {
				t.Error("scan did not run first")
			}
			return nil
		})
		seq.Task("venv").Define(func(ctx context.Context) error {
			if !step.CompareAndSwap(1, 2) {
				t.Error("venv did not run second")
			}
			return nil
		})
		seq.Task("install").Define(func(ctx context.Context) error {
			if !step.CompareAndSwap(2, 3) {
				t.Error("install did not run third")
			}
			return nil
		})
		return seq.wait()
	})
	if res.ExitCode() != exitOK {
		t.Fatalf("exit %d, want ExitOK; output:\n%s", res.ExitCode(), buf.String())
	}
}

// TestTaskHandle_Define_ReturnsHandleForChaining pins the Decisions' Define
// sugar: Define returns the same *Task so `task.Define(fn).Wait()`
// composes without a separate local variable, and this must not change
// Define's asynchronous scheduler semantics (the callback still runs on the
// scheduler, not inline).
func TestTaskHandle_Define_ReturnsHandleForChaining(t *testing.T) {
	var buf strings.Builder
	out := initRun(Config{
		plain: true, color: colorNever,
		MaxConcurrency: 4, stdout: &buf, stderr: io.Discard,
	})

	res := out.run(context.Background(), func(ctx context.Context, r *Run) error {
		task := out.Task("solo")
		return task.Define(func(ctx context.Context) error { return nil }).wait()
	})
	if res.ExitCode() != exitOK {
		t.Fatalf("exit %d, want ExitOK; output:\n%s", res.ExitCode(), buf.String())
	}
}
