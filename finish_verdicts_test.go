package evo

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// TestFinishEndsUndefinedTasksNotStarted pins the honest-verdict contract for
// plain (non-Group) parallel tasks: when Main's run returns an error while
// sibling tasks have no Define, Finish ends each NotStarted (CONTRACT.md §4)
// whether it only waited or also narrated work with Doing, because nothing of
// it ran; it never invents a Cancelled (■) row for work nobody started
// (evo-rec.md partial-truth rules).
func TestFinishEndsUndefinedTasksNotStarted(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})

	narrated := out.Task("download")
	narrated.Doing("fetching")
	pending := out.Sequence("steps").Task("verify")

	code := out.run(context.Background(), func(ctx context.Context, r *Run) error {
		return errors.New("boom")
	}).ExitCode()

	if code != exitFailed {
		t.Fatalf("exit code = %d, want ExitFailed (%d)", code, exitFailed)
	}
	for name, task := range map[string]*Task{"narrated": narrated, "pending": pending} {
		if state := task.snapshot().State; state != notStarted {
			t.Fatalf("%s task state = %q, want %q", name, state, notStarted)
		}
	}

	rendered := buf.String()
	if strings.Contains(rendered, "■") {
		t.Fatalf("rendered output draws a cancelled glyph (■) for work that never started:\n%s", rendered)
	}
	if !strings.Contains(rendered, "not started") {
		t.Fatalf("rendered output missing \"not started\" for the undefined tasks:\n%s", rendered)
	}
}
