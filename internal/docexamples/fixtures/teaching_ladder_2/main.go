// Package main compiles docs/guides/teaching-ladder.md's "Hosted (framework
// owns exit)" fence verbatim. See TestDocFencesMatchFixtures. Never run.
package main

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func run(ctx context.Context) error { return nil }

func doWork(ctx context.Context) {
	// docexamples:snippet start
	out := evo.Init(evo.Config{Title: "tool", Isolated: true})
	os.Exit(out.Run(ctx, run).ExitCode()) // reconciles a non-nil run error into Fail, then Finish
	// docexamples:snippet end
}

func main() { doWork(context.Background()) }
