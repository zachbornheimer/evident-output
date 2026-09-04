package evo_test

import (
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestInteractive_DebugTailReachesLiveTerminal is release-gate round 5
// finding 2: shouldPreserveDebugTailLocked's default preserveOnBad path only
// ever returns true when o.debugPaneActive is set — which only happens for a
// live rolling DebugPane, i.e. only in interactive mode. But writeDebugTail
// only used to render from residualPlainLocked, whose output reaches the
// live terminal solely through a distinct primary/AlsoWrite writer. The
// promised failure tail was therefore unreachable on the terminal a TTY user
// is actually watching. A DebugPane run that ends Failed must show its
// diagnostic tail on the interactive final render.
func TestInteractive_DebugTailReachesLiveTerminal(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Options: []evo.Option{
		evo.Terminal(screen), evo.VisibilityDelay(0), evo.NoColor(),
		evo.DebugLevel(evo.LevelDebug), evo.DebugPane(),
	}})
	t.Cleanup(func() { _ = out.Close() })

	out.Debug("diagnostic detail line")
	out.Task("build").Fail("compile error")
	_ = out.Finish()

	final := screen.FinalText()
	if !strings.Contains(final, "diagnostic detail line") {
		t.Fatalf("WriteFinal missing debug pane failure tail; got:\n%q", final)
	}
}
