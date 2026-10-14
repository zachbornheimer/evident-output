package evo

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/terminal"
)

// TestConfigToOptions_DefaultTTYMarksSharedPrimaryTerminal pins the
// construction-time plumbing: when Config's ordinary default-wiring builds
// a live Terminal around the same writer as primary (the real-TTY,
// no-explicit-to()/alsoWrite() path in (*evo.Run)(nil); _ = evo.Execute(context.Background(), evo.Config{Name: ...}, func(ctx context.Context, r *evo.Run) error { return nil })),
// configOf must record that identity so Finish can skip the
// dual-stream write, instead of an fd comparison happening later at Finish.
func TestConfigToOptions_DefaultTTYMarksSharedPrimaryTerminal(t *testing.T) {
	// /dev/null is a real *os.File backed by a character device, so
	// isCharDevice(c.Facades.Stdout) is true and configOf takes the same
	// "wantLive" auto-terminal branch a real interactive TTY would —
	// without requiring a pty in this test environment.
	tty, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = tty.Close() })

	built := resolveConfig(Config{Facades: Facades{Stdout: &bytes.Buffer{}, Stderr: tty}}, tuning{})
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
// configOf produces on a real TTY — one io.Writer serving as both
// primary (To) and the backing stream of the live terminal driver — and
// proves Finish renders the conclusion band exactly once through that
// writer, not once via the terminal's WriteFinal and again via the
// dual-stream residual write to primary.
func TestFinish_SharedPrimaryTerminalRendersConclusionOnce(t *testing.T) {
	var shared bytes.Buffer
	drv := terminal.NewANSI(&shared, terminal.WithInteractive(true), terminal.WithSize(80, 24))
	out := newOutput("shared-writer",
		to(&shared),
		withTerminal(drv),
		visibilityDelay(0),
		withNoColor(),
	)
	t.Cleanup(func() { _ = out.close() })

	out.Task("work").succeed("")
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	got := shared.String()
	if n := strings.Count(got, "[succeeded]"); n != 1 {
		t.Fatalf("conclusion band count = %d, want exactly 1 in:\n%s", n, got)
	}
}
