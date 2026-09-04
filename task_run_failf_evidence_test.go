package evo_test

import (
	"os/exec"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestRun_ThenFailf_RendersChildStderrInFinalReport is beginner-gate-2
// finding 3, root cause B: task_run.go's own documented spelling —
//
//	if err := task.Run(cmd); err != nil {
//	    return task.Failf("build failed: %w", err)
//	}
//
// — must render the failed child's captured stderr lines in the final
// report. Failf built its Problem from the wrapped error text alone
// (Problem.Detail), leaving no room for the task's retained Evidence to
// render: the auto-attach in finishTagged only fired when Detail was still
// empty, so a Failf's wrapped-error text silently occupied that slot and the
// child's own stderr — the actual proof of the failure — never appeared
// anywhere in the durable final report. Verified here on the interactive
// (live/TTY) rendering path via testkit.Screen — the path where this
// actually mattered, since plain-mode's per-line phase streaming can
// coincidentally echo the same text as a transient progress line before the
// bug ever reaches the final report — and, during the manual gate sweep,
// under a real pty (script(1)) capture too.
func TestRun_ThenFailf_RendersChildStderrInFinalReport(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(24), testkit.NoColor())
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build")
	cmd := exec.Command("/bin/sh", "-c", "echo 'undefined reference to main' 1>&2; exit 1")
	if err := task.Run(cmd); err != nil {
		_ = task.Failf("build failed: %w", err)
	}

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (a resolved Fail is not misuse)", err)
	}
	if state := out.Conclusion().State; state != evo.StateFailed {
		t.Fatalf("state = %v, want StateFailed", state)
	}
	final := screen.FinalText()
	if !strings.Contains(final, "undefined reference to main") {
		t.Fatalf("expected the child's stderr line in the durable final report, got:\n%s", final)
	}
	if !strings.Contains(final, "build failed") {
		t.Fatalf("expected the wrapped-error summary line too, got:\n%s", final)
	}
}
