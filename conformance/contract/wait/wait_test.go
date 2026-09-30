// Package wait_test binds contract §30 "Wait" rules to the public evo API.
package wait_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

var errProbe = errors.New("probe failure")

func newOutput(t *testing.T) *evo.Output {
	t.Helper()
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Color: evo.ColorNever,
		StateDir: t.TempDir(), Stdout: io.Discard, Stderr: io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })
	return out
}

func TestC30_026_WaitReturnsNilOnSuccessAndTheCallbackError(t *testing.T) {
	out := newOutput(t)
	ok := out.Task("ok").Define(func(context.Context) error { return nil })
	bad := out.Task("bad").Define(func(context.Context) error { return errProbe })
	if err := ok.Wait(); err != nil {
		t.Fatalf("Wait on a succeeded task = %v, want nil", err)
	}
	if err := bad.Wait(); !errors.Is(err, errProbe) {
		t.Fatalf("Wait on a failed task = %v, want the callback's error", err)
	}
}

func TestC30_030_WaitReturnsTheCancellation(t *testing.T) {
	out := newOutput(t)
	const reason = "superseded"
	task := out.Task("replaced")
	task.Cancel(reason)
	err := task.Wait()
	if err == nil || !strings.Contains(err.Error(), reason) {
		t.Fatalf("Wait on a cancelled task = %v, want a non-nil error carrying %q", err, reason)
	}
}

func TestC30_030_WaitOnAnInterruptedCallbackIsErrorsIsCanceled(t *testing.T) {
	out := newOutput(t)
	ctx, cancel := context.WithCancel(context.Background())
	var waitErr error
	out.Run(ctx, func(runCtx context.Context) error {
		slow := out.Task("slow")
		slow.Define(func(taskCtx context.Context) error {
			cancel()
			<-taskCtx.Done()
			return taskCtx.Err()
		})
		waitErr = slow.Wait()
		return waitErr
	})
	if !errors.Is(waitErr, context.Canceled) {
		t.Fatalf("Wait after the run was cancelled = %v, want errors.Is context.Canceled", waitErr)
	}
}

// waitOutcomes declares a failed producer, a dependent, and an independent
// task, waits on them in the given order, and returns each Wait's answer.
func waitOutcomes(t *testing.T, order []string) map[string]string {
	t.Helper()
	out := newOutput(t)
	producer := out.Task("producer").Define(func(context.Context) error { return errProbe })
	dependent := out.Task("dependent").After(producer)
	dependent.Define(func(context.Context) error { return nil })
	independent := out.Task("independent").Define(func(context.Context) error { return nil })
	handles := map[string]*evo.TaskHandle{"producer": producer, "dependent": dependent, "independent": independent}
	answers := map[string]string{}
	for _, name := range order {
		err := handles[name].Wait()
		switch {
		case err == nil:
			answers[name] = "nil"
		case errors.Is(err, evo.ErrNotStarted):
			answers[name] = "not-started"
		default:
			answers[name] = err.Error()
		}
	}
	return answers
}

func TestC30_036_WaitAnswerNeverDependsOnPriorWaits(t *testing.T) {
	orders := [][]string{
		{"producer", "dependent", "independent"},
		{"dependent", "independent", "producer"},
		{"independent", "dependent", "producer"},
	}
	want := waitOutcomes(t, orders[0])
	for _, order := range orders[1:] {
		got := waitOutcomes(t, order)
		for name, answer := range want {
			if got[name] != answer {
				t.Errorf("order %s: Wait(%s) = %q, want %q", strings.Join(order, ","), name, got[name], answer)
			}
		}
	}
}
