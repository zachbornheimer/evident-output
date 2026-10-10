package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/review"
)

// DOM-021: a Task declared in a function that never resolves it or hands
// it on stays unresolved, and the run concludes partial.

const writerOnlyTaskSrc = `package main

import (
	"context"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output"
)

func run(ctx context.Context) error {
	t := evo.Task("fetch")
	cmd := exec.Command("git", "fetch")
	cmd.Stdout = t.Writer()
	cmd.Stderr = t.Writer()
	return cmd.Run()
}
`

func TestDOM021_TaskNeverResolved_Fires(t *testing.T) {
	res := review.GoSource("main.go", writerOnlyTaskSrc)
	f := findingByID(t, res, "DOM-021")
	if f.Line != 11 {
		t.Fatalf("DOM-021 line = %d, want 11 (the declaration)", f.Line)
	}
	if !res.RecheckRequired {
		t.Fatal("DOM-021 finding did not require a recheck")
	}
}

const resolvedOrHandedOnTasksSrc = `package main

import (
	"context"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output"
)

func run(ctx context.Context) error {
	status := evo.Task("status")
	status.Define(func(ctx context.Context) error {
		cmd := exec.CommandContext(ctx, "git", "status")
		cmd.Stdout = status.Writer()
		return cmd.Run()
	})
	gate := evo.Task("gate")
	gate.Block("dirty tree")
	later := evo.Task("later")
	schedule(later)
	fetched := evo.Task("fetched").Define(fetch)
	fetched.Fact("remote", "origin")
	legacy := evo.Task("legacy") // pinned to a pre-1.1 release
	legacy.Delete("worktree", remove)
	ordered := evo.Task("ordered")
	ordered.After(fetched).Define(fetch)
	keyed := evo.Task("keyed")
	keyed.Key("build:main").Define(fetch)
	chained := evo.Task("chained")
	schedule(chained.Key("push"))
	return nil
}

func declare() *evo.TaskHandle {
	t := evo.Task("returned")
	return t
}

func schedule(*evo.TaskHandle)            {}
func fetch(context.Context) error        { return nil }
func remove() error                      { return nil }
`

func TestDOM021_ResolvedOrHandedOnTask_StaysSilent(t *testing.T) {
	res := review.GoSource("main.go", resolvedOrHandedOnTasksSrc)
	for _, f := range res.Findings {
		if f.RuleID == "DOM-021" {
			t.Fatalf("false positive DOM-021: %+v", f)
		}
	}
}

const chainedButUnresolvedTaskSrc = `package main

import evo "github.com/zachbornheimer/evident-output"

func run() {
	t := evo.Task("fetch")
	t.Key("fetch:origin").Fact("remote", "origin")
}
`

// TestDOM021_ChainWithoutResolvingLink_Fires proves the chain walk that
// accepts t.After(x).Define(fn) still reports a chain none of whose links
// resolve the Task or hand it on.
func TestDOM021_ChainWithoutResolvingLink_Fires(t *testing.T) {
	res := review.GoSource("main.go", chainedButUnresolvedTaskSrc)
	if f := findingByID(t, res, "DOM-021"); f.Line != 6 {
		t.Fatalf("DOM-021 line = %d, want 6 (the declaration)", f.Line)
	}
}
