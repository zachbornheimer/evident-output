// Package main compiles mcp/docs/teaching-ladder.md's "Hosted (framework
// owns exit)" fence verbatim. See TestDocFencesMatchFixtures. Never run.
package main

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func run(ctx context.Context, r *evo.Run) error { return nil }

func doWork(ctx context.Context) {
	// docexamples:snippet start
	os.Exit(evo.Execute(ctx, evo.Config{Name: "tool"}, run).ExitCode())
	// docexamples:snippet end
}

func main() { doWork(context.Background()) }
