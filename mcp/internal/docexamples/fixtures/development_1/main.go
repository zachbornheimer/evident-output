// Package main compiles mcp/docs/development.md's "Machine output" fence
// verbatim. See TestDocFencesMatchFixtures. Never run.
package main

import (
	"bytes"
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func doWork(ctx context.Context) {
	// docexamples:snippet start
	var buf bytes.Buffer
	_ = evo.Execute(ctx, evo.Config{Name: "tool", Output: evo.Plain, Facades: evo.Facades{Stderr: &buf}}, run)
	plain := buf.String() // rows and bands; Facades.Stdout receives only Run.ResultWriter
	// docexamples:snippet end

	_ = plain
}

func run(context.Context, *evo.Run) error { return nil }

func main() { doWork(context.Background()) }
