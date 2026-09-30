package engine

import (
	"context"
	"runtime"
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
		resultCh <- out.Run(context.Background(), func(context.Context) error {
			runtime.Goexit()
			return nil
		})
	}()

	select {
	case result := <-resultCh:
		if result.Conclusion.ExitCode != ExitFailed {
			t.Errorf("ExitCode = %d, want ExitFailed (%d)", result.Conclusion.ExitCode, ExitFailed)
		}
		if result.Err == nil {
			t.Fatal("Result.Err = nil, want an error explaining the callback exited without returning")
		}
	case <-time.After(goexitReturnTimeout):
		t.Fatal("Run did not return: run callback exited via runtime.Goexit and Run kept waiting")
	}
}
