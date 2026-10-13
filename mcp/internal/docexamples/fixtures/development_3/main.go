//go:build ignore

// Package main compiles mcp/docs/development.md's "Interactive (testkit /
// virtual terminal)" fence verbatim. See TestDocFencesMatchFixtures. Never
// run.
package main

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func doWork(ctx context.Context) {
	// docexamples:snippet start
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	_ = evo.Execute(ctx, evo.Config{
		Facades: evo.Facades{Terminal: screen, Clock: clock},
	}, run)
	// Phase/Progress draw a live region; a Task that resolves before the threshold does not flash.
	// docexamples:snippet end
}

func run(context.Context, *evo.Run) error { return nil }

func main() { doWork(context.Background()) }
