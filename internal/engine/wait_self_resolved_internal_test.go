package engine

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/graph"
)

// selfResolvedHold is how long the callback stays open after resolving its
// own row, so a waiter woken by the resolution has time to answer too early.
const selfResolvedHold = 20 * time.Millisecond

// TestTaskHandle_Wait_SelfResolvedBeforeReturn_IsNotSuccess pins the window
// between a callback resolving its own row (Block/Fail settle the row at
// once) and that callback actually returning (when its error is recorded). A
// waiter released inside that window used to see a Blocked row with no
// recorded error and report success — the race that made
// TestGroupHandle_Wait_BlockedChildSurfacesFailure hang under load. The
// callback is held open here after resolving, so the window is forced every
// run rather than hit one run in hundreds, and Wait must answer only once the
// callback has returned, with the error it returned.
func TestTaskHandle_Wait_SelfResolvedBeforeReturn_IsNotSuccess(t *testing.T) {
	syntax := errors.New("syntax")
	for _, tc := range []struct {
		name    string
		resolve func(task *TaskHandle) error
		wantErr func(error) bool
	}{
		{"Block", func(task *TaskHandle) error { task.Block("needs review", Detail("ambiguous")); return nil }, func(err error) bool { return errors.Is(err, graph.ErrWaitFailed) }},
		{"Fail", func(task *TaskHandle) error { task.Fail("compile", Detail("syntax")); return syntax }, func(err error) bool { return errors.Is(err, syntax) }},
		{"Block then error", func(task *TaskHandle) error { task.Block("needs review"); return syntax }, func(err error) bool { return errors.Is(err, syntax) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf strings.Builder
			out := Init(Config{
				Isolated: true, Plain: true, Color: ColorNever,
				MaxConcurrency: 4, Stdout: &buf, Stderr: io.Discard,
			})
			var waitErr error
			var returned atomic.Bool
			out.Run(context.Background(), func(ctx context.Context) error {
				resolved := make(chan struct{})
				task := out.Task("self-resolving")
				task.Define(func(ctx context.Context) error {
					err := tc.resolve(task)
					close(resolved)
					time.Sleep(selfResolvedHold)
					returned.Store(true)
					return err
				})
				<-resolved
				waitErr = task.Wait()
				return nil
			})
			if !returned.Load() {
				t.Error("Wait answered before the callback returned")
			}
			if !tc.wantErr(waitErr) {
				t.Errorf("Wait() = %v for a task its callback resolved %s; want the callback's own answer", waitErr, tc.name)
			}
		})
	}
}

// selfResolvedTrials repeats the no-hold race until a window a few
// instructions wide has been hit often enough to fail reliably when open.
const selfResolvedTrials = 200

// TestTaskHandle_Wait_FailOrBlockThenErrorAlwaysReturnsTheError pins the same
// window with no hold at all: Wait gives a wrong answer in none of the
// trials, whichever way the scheduler interleaves the settle and the return.
func TestTaskHandle_Wait_FailOrBlockThenErrorAlwaysReturnsTheError(t *testing.T) {
	boom := errors.New("boom")
	wrong := 0
	for range selfResolvedTrials {
		for _, resolve := range []func(*TaskHandle){
			func(task *TaskHandle) { task.Fail("f") },
			func(task *TaskHandle) { task.Block("b") },
		} {
			out := newOutput("job", to(io.Discard))
			task := out.Task("t")
			task.Define(func(context.Context) error { resolve(task); return boom })
			if err := task.Wait(); !errors.Is(err, boom) {
				wrong++
			}
			_ = out.Finish()
		}
	}
	if wrong > 0 {
		t.Errorf("Wait returned something other than the callback's error in %d/%d trials", wrong, 2*selfResolvedTrials)
	}
}
