package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/terminal"
)

// TestInteractive_MisuseHintReachesLiveTerminal is release-gate round 5
// finding 1: Finish appends the misuse hint line directly to o.lines
// (appendMisuseLineLocked) after every ordinary line has already streamed
// progressively. residualPlainLocked drains that unemitted tail for the
// plain/primary stream, but residualInteractiveFinalLocked never did — an
// interactive user got exit 2 with an all-green ledger and zero explanation
// on the actual terminal. A double-resolve (Block then Block again) records
// ErrAlreadyResolved; the corrective hint line must reach the live
// terminal's WriteFinal text, not just a primary/AlsoWrite mirror.
func TestInteractive_MisuseHintReachesLiveTerminal(t *testing.T) {
	var screen bytes.Buffer
	drv := terminal.NewANSI(&screen, terminal.WithInteractive(true), terminal.WithSize(80, 24))
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(drv), evo.VisibilityDelay(0), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("branches")
	task.Block("local-only branch")
	task.Block("second call ignored") // already resolved — misuse

	if err := out.Finish(); err == nil {
		t.Fatal("Finish() = nil, want the recorded misuse")
	}

	rendered := screen.String()
	const wantHint = "resolve each task once; branches was already resolved"
	if !strings.Contains(rendered, wantHint) {
		t.Fatalf("interactive terminal missing misuse hint %q; got:\n%q", wantHint, rendered)
	}
}
