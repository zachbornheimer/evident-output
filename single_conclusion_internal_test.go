package evo

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/terminal"
)

// TestConfigToOptions_DefaultTTYMarksSharedPrimaryTerminal pins the
// construction-time plumbing: when Config's ordinary default-wiring builds
// a live Terminal around the same writer as primary (the real-TTY,
// no-explicit-To()/AlsoWrite() path in evo.Init(evo.Config{Title: ...})),
// configToOptions must record that identity so Finish can skip the
// dual-stream write, instead of an fd comparison happening later at Finish.
func TestConfigToOptions_DefaultTTYMarksSharedPrimaryTerminal(t *testing.T) {
	// /dev/null is a real *os.File backed by a character device, so
	// IsCharDevice(c.Stdout) is true and configToOptions takes the same
	// "wantLive" auto-terminal branch a real interactive TTY would —
	// without requiring a pty in this test environment.
	tty, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = tty.Close() })

	cfg := resolveConfig(Config{Stdout: tty, Stderr: &bytes.Buffer{}})
	opts := configToOptions(cfg)

	var built config
	for _, o := range opts {
		o.apply(&built)
	}
	if built.terminal == nil {
		t.Fatal("expected an auto-built live Terminal driver on a char-device Stdout")
	}
	if built.primary != tty {
		t.Fatalf("expected primary to be the same writer as Stdout, got %v", built.primary)
	}
	if !built.samePrimaryAsTerminal {
		t.Fatal("expected samePrimaryAsTerminal=true when the default terminal is built around primary's writer")
	}
}

// TestFinish_SharedPrimaryTerminalRendersConclusionOnce is the red-first
// golden test for the double conclusion band: it reproduces the exact shape
// configToOptions produces on a real TTY — one io.Writer serving as both
// primary (To) and the backing stream of the live terminal driver — and
// proves Finish renders the conclusion band exactly once through that
// writer, not once via the terminal's WriteFinal and again via the
// dual-stream residual write to primary.
func TestFinish_SharedPrimaryTerminalRendersConclusionOnce(t *testing.T) {
	var shared bytes.Buffer
	drv := terminal.NewANSI(&shared, terminal.WithInteractive(true), terminal.WithSize(80, 24))
	out := newOutput("shared-writer",
		To(&shared),
		Terminal(drv),
		withPrimarySharesTerminal(),
		VisibilityDelay(0),
		NoColor(),
	)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("work").Done()
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	got := shared.String()
	if n := strings.Count(got, "[ready]"); n != 1 {
		t.Fatalf("conclusion band count = %d, want exactly 1 in:\n%s", n, got)
	}
}

// TestFinish_DistinctPrimaryAndTerminalKeepBothConclusions guards the
// deliberately-separate-streams contract (TestProgressive_InteractiveNoDoublePrint,
// TestDebugPane_FailurePreservesDiagnosticTail): when primary is a genuinely
// distinct writer from the live terminal's backing stream, Finish must keep
// writing the conclusion to primary — samePrimaryAsTerminal is never inferred,
// only ever set by configToOptions at construction.
func TestFinish_DistinctPrimaryAndTerminalKeepBothConclusions(t *testing.T) {
	var primary, screenBuf bytes.Buffer
	drv := terminal.NewANSI(&screenBuf, terminal.WithInteractive(true), terminal.WithSize(80, 24))
	out := newOutput("distinct-writers",
		To(&primary),
		Terminal(drv),
		VisibilityDelay(0),
		NoColor(),
	)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("work").Done()
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	if !strings.Contains(primary.String(), "[ready]") {
		t.Fatalf("distinct primary must still receive the conclusion:\n%s", primary.String())
	}
	if !strings.Contains(screenBuf.String(), "[ready]") {
		t.Fatalf("terminal must still receive the conclusion:\n%s", screenBuf.String())
	}
}
