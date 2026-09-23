// Package main compiles docs/reference.md's "file integrity" fence (one
// check Task, many Problems section) verbatim. See
// TestDocFencesMatchFixtures. Never run.
package main

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

type fileIssue struct {
	Summary string
	Path    string
	Code    string
	Line    int
}

func doWork() {
	out := evo.Init(evo.Config{Isolated: true})
	issues := []fileIssue{}

	// docexamples:snippet start
	task := out.Task("file integrity")
	for _, issue := range issues {
		task.Problem(issue.Summary,
			evo.On(issue.Path),
			evo.Code(issue.Code),
			evo.Location(issue.Path, issue.Line, 0),
		)
	}
	task.Define(func(context.Context) error { return nil })
	// docexamples:snippet end
}

func main() { doWork() }
