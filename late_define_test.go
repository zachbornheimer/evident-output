package evo_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// lateDefineRun is one run in which the Task "late" is Defined by a callback
// after the run body returned.
type lateDefineRun struct {
	run     contractRun
	lateRan bool
}

// runLateDefine declares "late" and optionally a Task After it, returns from
// the run body, and has a callback Define "late" once the drain has begun.
// The drain has begun when the JSONL stream first reports the dependent of
// "marker", a Task nobody Defines, as stopped by it.
func runLateDefine(t *testing.T, withDependent bool) lateDefineRun {
	t.Helper()
	var stdout, stderr lockedBuffer
	var lateRan atomic.Bool
	cfg := evo.Config{Name: "late", Output: evo.JSONL}
	cfg.Facades.Stdout, cfg.Facades.Stderr, cfg.Facades.Stdin = &stdout, &stderr, strings.NewReader("")
	body := func(_ context.Context, r *evo.Run) error {
		late := r.Task("late")
		marker := r.Task("marker")
		r.Task("after marker").After(marker).Define(func(context.Context) error { return nil })
		if withDependent {
			r.Task("after late").After(late).Define(func(context.Context) error { return nil })
		}
		r.Task("definer").Define(func(context.Context) error {
			deadline := time.Now().Add(contractRunDeadline / 2)
			for !markerNotStarted(stdout.String()) && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			late.Define(func(context.Context) error { lateRan.Store(true); return nil })
			return nil
		})
		return nil
	}
	done := make(chan evo.Result, 1)
	go func() { done <- evo.Execute(context.Background(), cfg, body) }()
	select {
	case res := <-done:
		return lateDefineRun{run: contractRun{Result: res, Stdout: stdout.String(), Stderr: stderr.String()}, lateRan: lateRan.Load()}
	case <-time.After(contractRunDeadline):
		t.Fatalf("Execute did not return within %s; output so far:\n%s%s", contractRunDeadline, stdout.String(), stderr.String())
		return lateDefineRun{}
	}
}

// markerNotStarted reports whether the JSONL stream so far has ended the
// Task After "marker" with "marker not started", the dependent's summary.
// That event is written when the drain begins and not before.
func markerNotStarted(stream string) bool {
	return strings.Contains(stream, "marker not started")
}

// TestLateDefineIsRefusedWithOrWithoutDependent pins one rule for CONTRACT
// section 7: a Define that arrives after the run body returned is refused,
// whether or not another Task waits After the Task it names. The refusal
// wraps ErrInvalidConfig, names the cause and the way out, and never asks
// for a Define the caller already gave. The two shapes end the same way.
func TestLateDefineIsRefusedWithOrWithoutDependent(t *testing.T) {
	shapes := map[string]bool{"with a dependent": true, "without a dependent": false}
	outcomes := map[string]string{}
	for name, withDependent := range shapes {
		t.Run(name, func(t *testing.T) {
			got := runLateDefine(t, withDependent)
			res, text := got.run.Result, got.run.Text()
			if !errors.Is(res.Err, evo.ErrInvalidConfig) {
				t.Errorf("Result.Err = %v, want it to wrap evo.ErrInvalidConfig", res.Err)
			}
			if res.ExitCode() != 2 {
				t.Errorf("exit code = %d, want 2", res.ExitCode())
			}
			if got.lateRan {
				t.Error("the late Define ran its callback, want it refused")
			}
			const remedy = "Define late before the run body returns, or declare it in a Build"
			if !strings.Contains(text, remedy) {
				t.Errorf("output lacks the remedy %q:\n%s", remedy, text)
			}
			for _, wrong := range []string{"resolve each task once", `give Task "late"`} {
				if strings.Contains(text, wrong) {
					t.Errorf("output holds %q, which names the wrong cause:\n%s", wrong, text)
				}
			}
			outcomes[name] = res.Conclusion.Explanation
		})
	}
	if with, without := outcomes["with a dependent"], outcomes["without a dependent"]; with != without {
		t.Errorf("explanations differ by shape:\n with a dependent:    %q\n without a dependent: %q", with, without)
	}
}
