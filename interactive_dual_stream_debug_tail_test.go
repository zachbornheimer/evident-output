package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestInteractive_DualStreamDebugTailReachesLiveTerminal reproduces the gate-5
// gap: TestInteractive_DebugTailReachesLiveTerminal (interactive_debug_tail_test.go)
// only exercises single-stream construction, where Diagnostics is never
// configured (or is configured to the same writer as primary) and
// projectDebugRecordLocked's dual early-return in output.go never fires.
//
// Ordinary Config-based construction — evo.Init(evo.Config{...}) with the
// realistic default Stdout/Stderr split — always sets Diagnostics to a
// different writer than primary (construct.go: To(c.Stdout), Diagnostics(
// c.Stderr)). Under that dual-stream shape, projectDebugRecordLocked wrote
// the record to Diagnostics and returned before ever setting
// debugPaneActive, so shouldPreserveDebugTailLocked's default preserveOnBad
// path (gated on debugPaneActive) could never fire — the failure tail
// promised by DebugPane was unreachable on the live terminal whenever the
// caller used two real, distinct streams instead of one.
//
// This harness gives Diagnostics and primary genuinely distinct writers (two
// real buffers, not testkit.Screen.Sink() which always returns nil) so the
// dual branch is actually exercised, while still using an interactive
// testkit.Screen as the live terminal driver.
func TestInteractive_DualStreamDebugTailReachesLiveTerminal(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	var primary, diagnostics bytes.Buffer

	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.Terminal(screen), evo.To(&primary), evo.Diagnostics(&diagnostics),
		evo.VisibilityDelay(0), evo.NoColor(),
		evo.DebugLevel(evo.LevelDebug), evo.DebugPane(),
	}})
	t.Cleanup(func() { _ = out.Close() })

	out.Debug("diagnostic detail line")
	out.Task("build").Fail("compile error")
	_ = out.Finish()

	if !strings.Contains(diagnostics.String(), "diagnostic detail line") {
		t.Fatalf("Diagnostics stream missing the debug record; got:\n%q", diagnostics.String())
	}

	final := screen.FinalText()
	if !strings.Contains(final, "diagnostic detail line") {
		t.Fatalf("dual-stream construction: live terminal missing debug pane failure tail; got:\n%q", final)
	}
}
