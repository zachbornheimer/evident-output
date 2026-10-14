package evo

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/terminal"
)

// TestInteractive_DryRunDeleteReachesLiveTerminalLedger is release-gate round
// 6 finding 1: residualInteractiveFinalLocked (the live terminal's WriteFinal
// body) never called writeEffects, so a TTY dry-run rendered "[planned]" with
// zero planned rows — the third parity gap between it and residualPlainLocked
// (after the misuse line and the debug tail in earlier rounds). A
// Task.Delete under Config.DryRun true must render its "[planned]
// delete N <object>" row on the real interactive terminal, not just a
// plain/primary mirror.
func TestInteractive_DryRunDeleteReachesLiveTerminalLedger(t *testing.T) {
	var screen bytes.Buffer
	drv := terminal.NewANSI(&screen, terminal.WithInteractive(true), terminal.WithSize(80, 24))
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, terminal: drv, visibilityDelay: DelayForTest(0), color: colorNever, DryRun: true})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("branches")
	task.Define(effectOf(Delete, "stale local branch", 3))

	if err := out.finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil", err)
	}

	rendered := screen.String()
	if !strings.Contains(rendered, "[planned]") {
		t.Fatalf("interactive terminal missing [planned] tag; got:\n%q", rendered)
	}
	if !strings.Contains(rendered, "delete") || !strings.Contains(rendered, "stale local branch") {
		t.Fatalf("interactive terminal missing the planned delete ledger row; got:\n%q", rendered)
	}
}

// TestInteractive_ChangesDeleteReachesLiveTerminalLedger covers the DryRun
// false half of the same gap: a real (not planned) Delete's "[changed]"
// ledger row must also reach the live terminal.
func TestInteractive_ChangesDeleteReachesLiveTerminalLedger(t *testing.T) {
	var screen bytes.Buffer
	drv := terminal.NewANSI(&screen, terminal.WithInteractive(true), terminal.WithSize(80, 24))
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, terminal: drv, visibilityDelay: DelayForTest(0), color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("branches")
	task.Define(effectOf(Delete, "stale local branch", 3))

	if err := out.finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil", err)
	}

	rendered := screen.String()
	if !strings.Contains(rendered, "[changed]") {
		t.Fatalf("interactive terminal missing [changed] tag; got:\n%q", rendered)
	}
	if !strings.Contains(rendered, "deleted") || !strings.Contains(rendered, "stale local branch") {
		t.Fatalf("interactive terminal missing the changed delete ledger row; got:\n%q", rendered)
	}
}
