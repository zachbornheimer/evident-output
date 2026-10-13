package evo

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// failMidLoop declares a Task that reached 3/10, recorded a Problem, then
// failed with its own summary.
func failMidLoop(task *Task) {
	task.Define(func(context.Context) error {
		task.Progress(3, 10)
		task.Problem("exit status 2")
		task.fail("compile stopped")
		return nil
	})
}

// TestTaskRow_SameHeadlineAtRootAndUnderAHeader pins one row policy: a
// Task's row states the same headline (its Summary, with the count it
// reached) whether it sits at the root or under a Sequence header.
func TestTaskRow_SameHeadlineAtRootAndUnderAHeader(t *testing.T) {
	render := func(declare func(out *Run)) string {
		var buf bytes.Buffer
		out := initRun(nonTTYConfig("tool", &buf))
		t.Cleanup(func() { _ = out.close() })
		declare(out)
		_ = out.finish()
		return buf.String()
	}
	root := render(func(out *Run) { failMidLoop(out.Task("build")) })
	nested := render(func(out *Run) { failMidLoop(out.Sequence("steps").Task("build")) })

	const row = "✗ build  3/10  compile stopped\n"
	if !strings.Contains(root, row) {
		t.Fatalf("root row must state the Summary with its count:\n%s", root)
	}
	if !strings.Contains(nested, "   "+row) {
		t.Fatalf("a row under a header must state the same headline as at the root:\n%s", nested)
	}
	if !strings.Contains(nested, "exit status 2") {
		t.Fatalf("the other Problem still renders under the row:\n%s", nested)
	}
}
