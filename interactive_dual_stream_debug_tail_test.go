package evo

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/testkit"
)

// sinkScreen is an interactive testkit.Screen whose driver reports sink as its destination, so the run's rows
// reach sink while stderr stays a distinct diagnostic stream.
type sinkScreen struct {
	*testkit.Screen
	sink io.Writer
}

func (s sinkScreen) Sink() io.Writer { return s.sink }

// TestInteractive_DualStreamDebugTailReachesLiveTerminal reproduces the gate-5
// gap: with a terminal whose sink is a stream other than stderr, debug records
// go to stderr alone, and the record that went there must still set the pane
// active, or the failure tail DebugPane promises never reaches the live terminal.
func TestInteractive_DualStreamDebugTailReachesLiveTerminal(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	var primary, diagnostics bytes.Buffer

	newest := true
	out := initRun(Config{Facades: Facades{Stdout: io.Discard, Stderr: &diagnostics, Terminal: sinkScreen{screen, &primary}}},
		tuning{visibilityDelay: DelayForTest(0), debug: debugConfig{level: levelDebug, view: debugPresentationPane, paneHeight: 5, newestFirst: &newest}, color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	out.debugForTest("diagnostic detail line")
	out.Task("build").fail("compile error")
	_ = out.finish()

	if !strings.Contains(diagnostics.String(), "diagnostic detail line") {
		t.Fatalf("stderr missing the debug record; got:\n%q", diagnostics.String())
	}

	final := screen.FinalText()
	if !strings.Contains(final, "diagnostic detail line") {
		t.Fatalf("dual-stream construction: live terminal missing debug pane failure tail; got:\n%q", final)
	}
}
