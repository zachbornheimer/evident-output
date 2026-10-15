// Package main compiles mcp/docs/reference.md's "file integrity" fence (one
// check Task, many Problems section) verbatim. See
// TestDocFencesMatchFixtures. Never run.
package main

import (
	"context"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

type fileIssue struct {
	Summary string
	Path    string
	Code    string
	Line    int
}

var issues = []fileIssue{}

// docexamples:snippet start
func run(ctx context.Context, r *evo.Run) error {
	task := r.Task("check file integrity")
	for _, issue := range issues {
		task.Problem(issue.Summary,
			evo.On(fmt.Sprintf("%s:%d", issue.Path, issue.Line)),
			evo.Code(issue.Code),
		)
	}
	task.Define(func(context.Context) error {
		if len(issues) > 0 {
			return evo.Fail("file integrity check found problems")
		}
		return nil
	})
	return nil
}

// docexamples:snippet end

func main() { os.Exit(evo.Main(evo.Config{}, run)) }
