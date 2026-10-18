package evo

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/testkit"
)

// TestGroupOwnTaskFoldsIntoItsRow proves a Group's own Task (the Task named
// for its Group) is the Group's row, not a second row under it: the name is
// printed once, the own Task's summary rides that row, and the other items
// nest beneath it, in the durable output and the finished live frame.
func TestGroupOwnTaskFoldsIntoItsRow(t *testing.T) {
	declare := func(o *Run) *Group {
		g := o.Group("branches")
		g.Task("branches").Summary("3 stale").Define(func(context.Context) error { return nil })
		for i := range 2 {
			g.Task(fmt.Sprintf("b%d", i)).Define(func(context.Context) error { return nil })
		}
		return g
	}

	t.Run("plain", func(t *testing.T) {
		got, _ := runPlain(t, Config{}, func(o *Run) { declare(o) })
		assertFrame(t, got, "✓ branches  3 stale\n   ✓ b0\n   ✓ b1\n\n[succeeded]  "+testProgramName+"\n")
	})

	t.Run("live", func(t *testing.T) {
		screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(24), testkit.NoColor())
		out := initRun(Config{Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Clock: testkit.NewClock(), Terminal: screen}}, tuning{visibilityDelay: DelayForTest(0), color: colorNever, maxFrameRate: 1_000_000})
		t.Cleanup(func() { _ = out.close() })
		_ = declare(out).wait()
		if got, want := strings.TrimRight(screen.LatestLiveText(), "\n"), "✓ branches  3 stale\n   ✓ b0\n   ✓ b1"; got != want {
			t.Errorf("finished frame:\n--- want ---\n%s\n--- got ---\n%s", want, got)
		}
	})
}
