package evo_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// failMidLoop declares a Task that reached 3/10, recorded a Problem, then
// failed with its own summary.
func failMidLoop(task *evo.TaskHandle) {
	task.Define(func(context.Context) error {
		task.Progress(3, 10)
		task.Problem("exit status 2")
		task.Fail("compile stopped")
		return nil
	})
}

// TestTaskRow_SameHeadlineAtRootAndUnderAHeader pins one row policy: a
// Task's row states the same headline (its Summary, with the count it
// reached) whether it sits at the root or under a Sequence header.
func TestTaskRow_SameHeadlineAtRootAndUnderAHeader(t *testing.T) {
	render := func(declare func(out *evo.Output)) string {
		var buf bytes.Buffer
		out := evo.Init(nonTTYConfig("tool", &buf))
		t.Cleanup(func() { _ = out.Close() })
		declare(out)
		_ = out.Finish()
		return buf.String()
	}
	root := render(func(out *evo.Output) { failMidLoop(out.Task("build")) })
	nested := render(func(out *evo.Output) { failMidLoop(out.Sequence("steps").Task("build")) })

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
