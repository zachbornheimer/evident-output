// Package main compiles docs/development.md's "Interactive (testkit /
// virtual terminal)" fence verbatim. See TestDocFencesMatchFixtures. Never
// run.
package main

import (
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func doWork() {
	// docexamples:snippet start
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Options: []evo.Option{
		evo.Terminal(screen),
		evo.Clock(clock),
		evo.VisibilityDelay(150 * time.Millisecond),
		evo.MaxFrameRate(20),
	}})
	// Phase/Progress draw a live region; instant Done before the threshold does not flash.
	// DebugHistory (default): out.Debug → durable above live (timestamp + [DEBUG]).
	// DebugPane(...): rolling slog viewport in the live region; optional failure tail.
	// docexamples:snippet end

	_ = out
}

func main() { doWork() }
