package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/terminal"
)

// TestInteractive_DryRunDeleteReachesLiveTerminalLedger is release-gate round
// 6 finding 1: residualInteractiveFinalLocked (the live terminal's WriteFinal
// body) never called writeEffects, so a TTY dry-run rendered "[planned]" with
// zero planned rows — the third parity gap between it and residualPlainLocked
// (after the misuse line and the debug tail in earlier rounds). A
// TaskHandle.Delete under Config.DryRun true must render its "[planned]
// delete N <object>" row on the real interactive terminal, not just a
// plain/primary mirror.
func TestInteractive_DryRunDeleteReachesLiveTerminalLedger(t *testing.T) {
	var screen bytes.Buffer
	drv := terminal.NewANSI(&screen, terminal.WithInteractive(true), terminal.WithSize(80, 24))
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.Terminal(drv), evo.VisibilityDelay(0), evo.NoColor(), evo.DryRun(),
	}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("branches")
	_ = task.Delete("stale local branch", nil, evo.Affected(3))
	task.Done()

	if err := out.Finish(); err != nil {
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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.Terminal(drv), evo.VisibilityDelay(0), evo.NoColor(),
	}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("branches")
	_ = task.Delete("stale local branch", nil, evo.Affected(3))
	task.Done()

	if err := out.Finish(); err != nil {
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
