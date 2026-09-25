package evo_test

import (
	"io"
	"os/exec"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestRun_ThenFail_RendersChildStderrInFinalReport is beginner-gate-2
// finding 3, root cause B: a statement-form Fail with no explicit Detail
// must still render the failed child's captured stderr in the final report
// — auto-attach fills the Problem's EvidenceTail from the Task's retained
// evidence, the paved path 1.1 uses in place of TaskHandle.Failf's own
// automatic evidence attachment (Failf is removed with no compatibility
// alias). Verified on the interactive (live/TTY) rendering path via
// testkit.Screen, where plain-mode's per-line phase streaming cannot
// coincidentally echo the same text as a transient progress line.
func TestRun_ThenFail_RendersChildStderrInFinalReport(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(24), testkit.NoColor())
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build")
	cmd := exec.Command("/bin/sh", "-c", "echo 'undefined reference to main' 1>&2; exit 1")
	if err := task.RunForTest(cmd); err != nil {
		task.Fail("build failed")
	}

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (a resolved Fail is not misuse)", err)
	}
	if state := out.Conclusion().State; state != evo.StateFailed {
		t.Fatalf("state = %v, want StateFailed", state)
	}
	// Fail commits the resolved row durably at resolution time
	// (release-gate round 5 finding 3, commitResolvedTaskLocked) rather than
	// waiting for WriteFinal — PersistedText covers both durable and final.
	persisted := screen.PersistedText()
	if !strings.Contains(persisted, "undefined reference to main") {
		t.Fatalf("expected the child's stderr line in the durable report, got:\n%s", persisted)
	}
	if !strings.Contains(persisted, "build failed") {
		t.Fatalf("expected the summary line too, got:\n%s", persisted)
	}
}
