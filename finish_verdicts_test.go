package evo

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// TestFinishResolvesRunningTaskAsCancelled pins the honest-verdict contract for
// plain (non-Group) parallel tasks: when Main's run returns an error while
// sibling tasks are still Running/Pending, Finish must resolve each task to a
// real terminal state derived from the one model — Running becomes Cancelled
// (■), Pending becomes "not started" — never a stuck/incomplete glyph that
// forces the consumer to hand-roll the same cascade (evo-rec.md partial-truth
// rules).
func TestFinishResolvesRunningTaskAsCancelled(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &buf, color: colorNever, plain: true})

	running := out.Task("download")
	running.Doing("fetching")
	pending := out.Sequence("steps").Task("verify")

	code := out.run(context.Background(), func(ctx context.Context) error {
		return errors.New("boom")
	}).ExitCode()

	if code != exitFailed {
		t.Fatalf("exit code = %d, want ExitFailed (%d)", code, exitFailed)
	}

	runningState := running.snapshot().State
	if runningState != Cancelled {
		t.Fatalf("running task state = %q, want %q", runningState, Cancelled)
	}

	pendingState := pending.snapshot().State
	if pendingState != notStarted {
		t.Fatalf("pending task state = %q, want %q", pendingState, notStarted)
	}

	rendered := buf.String()
	if !strings.Contains(rendered, "■") {
		t.Fatalf("rendered output missing cancelled glyph (■):\n%s", rendered)
	}
	if !strings.Contains(rendered, "not started") {
		t.Fatalf("rendered output missing \"not started\" for pending task:\n%s", rendered)
	}
}
