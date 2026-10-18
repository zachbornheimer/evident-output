package evo

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

const goexitReturnTimeout = 3 * time.Second

// TestRun_ReturnsWhenRunCallbackExitsViaGoexit pins that a run callback whose
// goroutine ends through runtime.Goexit (directly, or via a library such as
// testing.T.FailNow) still concludes the run as Failed instead of leaving Run
// waiting forever for a result that will never be sent.
func TestRun_ReturnsWhenRunCallbackExitsViaGoexit(t *testing.T) {
	out := newTestOutput(t)

	resultCh := make(chan Result, 1)
	go func() {
		resultCh <- out.run(context.Background(), func(ctx context.Context, r *Run) error {
			runtime.Goexit()
			return nil
		})
	}()

	select {
	case result := <-resultCh:
		if result.Conclusion.exitCode != exitFailed {
			t.Errorf("ExitCode = %d, want ExitFailed (%d)", result.Conclusion.exitCode, exitFailed)
		}
		if result.Err == nil {
			t.Fatal("Result.Err = nil, want an error explaining the callback exited without returning")
		}
	case <-time.After(goexitReturnTimeout):
		t.Fatal("Run did not return: run callback exited via runtime.Goexit and Run kept waiting")
	}
}

// TestGoexitCallbackReadsAsItsOwnEnding pins that a Define callback ending
// through runtime.Goexit fails its own row with what happened, instead of
// reading as cancelled because "the run concluded before finish".
func TestGoexitCallbackReadsAsItsOwnEnding(t *testing.T) {
	var stderr bytes.Buffer
	result := Execute(context.Background(), Config{Output: Plain, Facades: Facades{Stderr: &stderr}}, func(ctx context.Context, r *Run) error {
		r.Task("quits").Define(func(context.Context) error {
			runtime.Goexit()
			return nil
		})
		r.Task("after").Define(func(context.Context) error { return nil })
		return nil
	})

	if got := result.ExitCode(); got != int(exitFailed) {
		t.Errorf("exit code = %d, want ExitFailed (%d)", got, exitFailed)
	}
	rendered := stderr.String()
	for _, want := range []string{"quits", "callback exited without returning (runtime.Goexit)"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("output missing %q:\n%s", want, rendered)
		}
	}
	for _, blame := range []string{"cancelled", "run concluded before finish"} {
		if strings.Contains(rendered, blame) {
			t.Errorf("output blames the run with %q:\n%s", blame, rendered)
		}
	}
}
