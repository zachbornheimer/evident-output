package evo

import (
	"io"
	"strings"
	"testing"
	"time"
)

// TestRun_Interrupt_DeclaredButNeverDefinedTaskIsNotMisuse is the red-first
// proof for the canary's `cancel-pending` shape: three subjects each declare
// one explicitly named `classify` child, the domain narrates progress into
// one of them, and ^C arrives. The narrated row was cancelled correctly, but
// the two that were never Defined stayed Pending — abandonQueuedWork only
// swept tasks the scheduler had accepted — so Finish recorded
// ErrUnresolvedTask and told the user to "call Done, Fail, Block, Skipped,
// or a mutation verb on this task" about work an interrupt had just taken
// away from them.
//
// An interrupt's answer to "and what about the rest?" is the same for a
// queued task and a declared one: it never started.
func TestRun_Interrupt_DeclaredButNeverDefinedTaskIsNotMisuse(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{
		Isolated: true, Plain: true, Color: ColorNever,
		MaxConcurrency: 1, Stdout: &buf, Stderr: io.Discard,
	})
	interrupt := sendOneSignal(t)

	running := out.Group("worktrees").Task("classify")
	out.Group("branches").Task("classify")
	out.Group("remote-tracking").Task("classify")

	blocking := make(chan struct{})
	code := make(chan int, 1)
	go func() {
		code <- out.Run(func(o *Output) error {
			running.Progress(24, 111)
			close(blocking)
			<-running.Context().Done()
			return running.Context().Err()
		})
	}()

	<-blocking
	interrupt()

	select {
	case got := <-code:
		if got != ExitCancelled {
			t.Fatalf("exit %d, want %d; output:\n%s", got, ExitCancelled, buf.String())
		}
	case <-time.After(interruptBudget):
		t.Fatalf("one interrupt did not stop the run; output so far:\n%s", buf.String())
	}

	rendered := buf.String()
	if strings.Contains(rendered, "call Done, Fail, Block") {
		t.Fatalf("an interrupt is not the caller's bookkeeping error:\n%s", rendered)
	}
	if strings.Contains(rendered, "not resolved") || strings.Contains(rendered, "unresolved") {
		t.Fatalf("an interrupted run must record no unresolved-task misuse:\n%s", rendered)
	}
	if n := strings.Count(rendered, "not started"); n != 2 {
		t.Fatalf("want both never-defined subjects accounted for as not started, got %d:\n%s", n, rendered)
	}
	if !strings.Contains(rendered, "■ ") {
		t.Fatalf("the narrated row still says it was interrupted:\n%s", rendered)
	}
}
