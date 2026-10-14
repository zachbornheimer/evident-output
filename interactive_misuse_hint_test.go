package evo

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/terminal"
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
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, terminal: drv, visibilityDelay: DelayForTest(0), color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("branches")
	task.block("local-only branch")
	task.block("second call ignored") // already resolved — misuse

	if err := out.finish(); err == nil {
		t.Fatal("Finish() = nil, want the recorded misuse")
	}

	rendered := screen.String()
	const wantHint = "resolve each task once; branches was already resolved"
	if !strings.Contains(rendered, wantHint) {
		t.Fatalf("interactive terminal missing misuse hint %q; got:\n%q", wantHint, rendered)
	}
}
