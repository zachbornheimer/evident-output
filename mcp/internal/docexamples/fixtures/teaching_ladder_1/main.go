// Package main compiles mcp/docs/teaching-ladder.md's "Standalone
// (package-level default instance)" fence verbatim. See
// TestDocFencesMatchFixtures. Never run.
package main

// The doc fence below assumes these three imports are already in
// scope (it is a continuation of the ladder's earlier "package-level
// default instance" narration); declared as fixture scaffolding here
// so the marked region matches the doc byte-for-byte.
import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

// docexamples:snippet start
func main() {
	os.Exit(evo.Main(evo.Config{}, run))
}

func run(ctx context.Context, r *evo.Run) error {
	worktrees := r.Group("worktrees")
	for _, path := range items {
		worktrees.Task(path).Define(func(ctx context.Context) error { return check(path) })
	}
	return nil
}

// docexamples:snippet end

// The fenced block above references these two names without defining them
// (the doc leaves them to the reader's own program); the fixture supplies
// trivial stand-ins purely so the snippet type-checks.
var items = []string{"one"}

func check(path string) error { return nil }
