// Package main compiles mcp/docs/reference.md's "file integrity" fence (one
// check Task, many Problems section) verbatim. See
// TestDocFencesMatchFixtures. Never run.
package main

import (
	"context"
	"fmt"

	evo "github.com/zachbornheimer/evident-output"
)

type fileIssue struct {
	Summary string
	Path    string
	Code    string
	Line    int
}

func doWork(out *evo.Run) {
	issues := []fileIssue{}

	// docexamples:snippet start
	task := out.Task("check file integrity")
	for _, issue := range issues {
		task.Problem(issue.Summary,
			evo.On(fmt.Sprintf("%s:%d", issue.Path, issue.Line)),
			evo.Code(issue.Code),
		)
	}
	task.Define(func(context.Context) error { return nil })
	// docexamples:snippet end
}

func main() { doWork(nil) }
