package review_test

import (
	"slices"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// DOM-021: a Task declared in a function that never Defines it or hands
// it on never runs, and the run concludes partial.

const writerOnlyTaskSrc = `package main

import (
	"context"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(ctx context.Context, r *evo.Run) error {
	t := r.Task("fetch")
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

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(ctx context.Context, r *evo.Run) error {
	status := r.Task("status")
	status.Define(func(ctx context.Context) error {
		cmd := exec.CommandContext(ctx, "git", "status")
		cmd.Stdout = status.Writer()
		return cmd.Run()
	})
	later := r.Task("later")
	schedule(later)
	fetched := r.Task("fetched").Define(fetch)
	fetched.Fact("remote", "origin")
	ordered := r.Task("ordered")
	ordered.After(fetched).Define(fetch)
	chained := r.Task("chained")
	schedule(chained.Summary("push"))
	produced := r.Task("produced")
	_ = evo.Compute(produced, produce)
	return nil
}

func declare(r *evo.Run) *evo.Task {
	t := r.Task("returned")
	return t
}

func schedule(*evo.Task)                 {}
func fetch(context.Context) error        { return nil }
func produce(context.Context) (int, error) { return 0, nil }
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

import evo "github.com/zachbornheimer/evident-output/v2"

func run(r *evo.Run) {
	t := r.Task("fetch")
	t.Summary("fetch origin").Fact("remote", "origin")
}
`

// TestDOM021_ChainWithoutResolvingLink_Fires proves the chain walk that
// accepts t.After(x).Define(fn) still reports a chain none of whose links
// Define the Task or hand it on.
func TestDOM021_ChainWithoutResolvingLink_Fires(t *testing.T) {
	res := review.GoSource("main.go", chainedButUnresolvedTaskSrc)
	if f := findingByID(t, res, "DOM-021"); f.Line != 6 {
		t.Fatalf("DOM-021 line = %d, want 6 (the declaration)", f.Line)
	}
}

const undefinedTaskStatementsSrc = `package main

import evo "github.com/zachbornheimer/evident-output/v2"

func run(r *evo.Run) {
	r.Task("bare")
	r.Task("fact").Fact("k", "v")
	_ = r.Task("discarded")
	var declared = r.Task("declared")
	var typed *evo.Task
	typed = r.Task("assigned")
}
`

// TestDOM021FiresOnBareTaskStatement pins that DOM-021 reports a Task no
// Define reaches whatever form its declaration takes: a statement that binds
// nothing, a blank assignment, a var, a plain assignment.
func TestDOM021FiresOnBareTaskStatement(t *testing.T) {
	res := review.GoSource("main.go", undefinedTaskStatementsSrc)
	var lines []int
	for _, f := range res.Findings {
		if f.RuleID == "DOM-021" {
			lines = append(lines, f.Line)
		}
	}
	want := []int{6, 7, 8, 9, 11}
	if !slices.Equal(lines, want) {
		t.Fatalf("DOM-021 lines = %v, want %v (bare, fact, discarded, declared, assigned)", lines, want)
	}
}

const definedOrHandedOnStatementsSrc = `package main

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run, tasks map[string]*evo.Task, keep chan *evo.Task) *evo.Task {
	r.Task("defined").Define(func(context.Context) error { return nil })
	r.Task("chained").Fact("k", "v").Define(func(context.Context) error { return nil })
	tasks["stored"] = r.Task("stored")
	keep <- r.Task("sent")
	schedule(r.Task("passed"))
	return r.Task("returned")
}

func schedule(*evo.Task) {}
`

func TestDOM021StaysSilentOnDefinedOrHandedOnStatements(t *testing.T) {
	res := review.GoSource("main.go", definedOrHandedOnStatementsSrc)
	for _, f := range res.Findings {
		if f.RuleID == "DOM-021" {
			t.Fatalf("false positive DOM-021: %+v", f)
		}
	}
}
