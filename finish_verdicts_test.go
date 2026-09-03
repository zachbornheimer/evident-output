package evo_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
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
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor())

	running := out.Task("download")
	running.Phase("fetching")
	pending := out.Task("verify")

	code := evo.MainWith(out, func(o *evo.Output) error {
		return errors.New("boom")
	})

	if code != evo.ExitFailed {
		t.Fatalf("exit code = %d, want ExitFailed (%d)", code, evo.ExitFailed)
	}

	runningState := running.Snapshot().State
	if runningState != evo.Cancelled {
		t.Fatalf("running task state = %q, want %q", runningState, evo.Cancelled)
	}

	pendingState := pending.Snapshot().State
	if pendingState != evo.NotStarted {
		t.Fatalf("pending task state = %q, want %q", pendingState, evo.NotStarted)
	}

	rendered := buf.String()
	if !strings.Contains(rendered, "■") {
		t.Fatalf("rendered output missing cancelled glyph (■):\n%s", rendered)
	}
	if !strings.Contains(rendered, "not started") {
		t.Fatalf("rendered output missing \"not started\" for pending task:\n%s", rendered)
	}
}
