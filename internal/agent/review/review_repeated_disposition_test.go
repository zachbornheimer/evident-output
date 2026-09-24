package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// API-062: Kept/Skipped record the Task's own disposition (the item is the
// Task, docs/reference.md) and resolve it, so a second call on the same
// Task is misuse. The per-item shape is group.Task(item).Kept(reason); the
// renderer folds those children into one tally (contract §25).

// zq prune's pre-fix shape: Kept once per kept branch, on the category Task.
const keptInLoopSrc = `package p

import evo "github.com/zachbornheimer/evident-output"

func define(task *evo.TaskHandle, locals []decision) {
	for _, d := range locals {
		if !d.Delete {
			task.Kept(keepReason(d.Reason))
		}
	}
}
`

func TestAPI062_KeptInLoopOnOneTask_Fires(t *testing.T) {
	f := findingByID(t, review.GoSource("clean.go", keptInLoopSrc), "API-062")
	if !strings.Contains(f.Suggestion, ".Task(") || !strings.Contains(f.Suggestion, ".Kept(") {
		t.Fatalf("API-062 suggestion must spell group.Task(item).Kept(reason): %q", f.Suggestion)
	}
}

const skippedTwiceSrc = `package p

import evo "github.com/zachbornheimer/evident-output"

func run(out *evo.Output) {
	task := out.Task("branches")
	task.Skipped(evo.Reason("protected"))
	task.Skipped(evo.Reason("dirty"))
}
`

func TestAPI062_SkippedTwiceInSequence_Fires(t *testing.T) {
	findingByID(t, review.GoSource("skip.go", skippedTwiceSrc), "API-062")
}

// The fix: one child Task per item, each resolving its own disposition.
const perItemKeptSrc = `package p

import evo "github.com/zachbornheimer/evident-output"

func define(branches *evo.GroupHandle, locals []decision) {
	for _, d := range locals {
		if !d.Delete {
			branches.Task(d.Name).Kept(keepReason(d.Reason))
		}
	}
	for _, d := range locals {
		item := branches.Task(d.Name)
		item.Kept(keepReason(d.Reason))
	}
}
`

func TestAPI062_PerItemKept_Silent(t *testing.T) {
	assertNoFinding(t, review.GoSource("clean.go", perItemKeptSrc), "API-062")
}

// Two different Tasks, or one Task whose two dispositions sit on exclusive
// branches, record once each.
const distinctOrExclusiveSrc = `package p

import evo "github.com/zachbornheimer/evident-output"

func run(out *evo.Output, skip bool) {
	a, b := out.Task("a"), out.Task("b")
	a.Kept(evo.Reason("protected"))
	b.Kept(evo.Reason("protected"))
	remote := out.Task("remote-tracking")
	if skip {
		remote.Skipped(evo.Reason("--skip-fetch"))
	} else {
		remote.Kept(evo.Reason("offline"))
	}
}
`

func TestAPI062_DistinctTasksOrExclusiveBranches_Silent(t *testing.T) {
	assertNoFinding(t, review.GoSource("run.go", distinctOrExclusiveSrc), "API-062")
}
