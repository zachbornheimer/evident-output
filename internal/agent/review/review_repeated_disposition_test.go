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

// A receiver rebound by plain assignment is a fresh Task each time: once
// per loop iteration, or again before a second call in one statement list.
const reboundPerItemSrc = `package p

import evo "github.com/zachbornheimer/evident-output"

func define(branches *evo.GroupHandle, locals []decision) {
	var item *evo.TaskHandle
	for _, d := range locals {
		item = branches.Task(d.Name)
		item.Kept(keepReason(d.Reason))
	}
	item = branches.Task("main")
	item.Kept(evo.Reason("protected"))
	item = branches.Task("develop")
	item.Kept(evo.Reason("protected"))
}
`

func TestAPI062_ReceiverReboundPerItem_Silent(t *testing.T) {
	assertNoFinding(t, review.GoSource("rebound.go", reboundPerItemSrc), "API-062")
}

// zq prune's shape: categories x items. The item Task is bound inside the
// inner loop, so each Kept resolves its own fresh Task; the outer loop must
// not read the inner binding as missing.
const nestedPerItemSrc = `package p

import evo "github.com/zachbornheimer/evident-output"

func define(groups map[string][]string, reason evo.ReasonValue) {
	for name, items := range groups {
		g := evo.Group(name)
		for _, item := range items {
			t := g.Task(item)
			t.Kept(reason)
		}
		for i := 0; i < len(items); i++ {
			u := g.Task(items[i])
			u.Skipped(reason)
		}
	}
}
`

func TestAPI062_NestedLoopPerItem_Silent(t *testing.T) {
	assertNoFinding(t, review.GoSource("nested.go", nestedPerItemSrc), "API-062")
}

// A Task bound once in the outer loop but resolved on every inner
// iteration still repeats.
const nestedRepeatSrc = `package p

import evo "github.com/zachbornheimer/evident-output"

func define(groups map[string][]string, reason evo.ReasonValue) {
	for name, items := range groups {
		t := evo.Task(name)
		for range items {
			t.Kept(reason)
		}
	}
}
`

func TestAPI062_NestedLoopOneTask_Fires(t *testing.T) {
	findingByID(t, review.GoSource("nested.go", nestedRepeatSrc), "API-062")
}

// A disposition call in a block that leaves the loop (return, goto, or a
// break out of the loop itself) runs at most once, however many items the
// loop visits.
const dispositionThenLeaveLoopSrc = `package p

import evo "github.com/zachbornheimer/evident-output"

func define(task *evo.TaskHandle, repos []repo) error {
	for _, r := range repos {
		if r.Dirty {
			task.Kept(evo.Reason("dirty"))
			return nil
		}
	}
	for _, r := range repos {
		if r.Protected {
			if r.Main {
				task.Skipped(evo.Reason("protected"))
			}
			break
		}
	}
	for _, r := range repos {
		if r.Locked {
			task.Skipped(evo.Reason("locked"))
			goto done
		}
	}
done:
	return nil
}
`

func TestAPI062_DispositionThenLeaveLoop_Silent(t *testing.T) {
	assertNoFinding(t, review.GoSource("leave.go", dispositionThenLeaveLoopSrc), "API-062")
}

// A break inside a switch leaves only the switch, so the loop still runs
// the disposition once per item.
const dispositionThenBreakSwitchSrc = `package p

import evo "github.com/zachbornheimer/evident-output"

func define(task *evo.TaskHandle, repos []repo) {
	for _, r := range repos {
		switch {
		case r.Dirty:
			task.Kept(evo.Reason("dirty"))
			break
		}
	}
}
`

func TestAPI062_DispositionThenBreakSwitch_Fires(t *testing.T) {
	findingByID(t, review.GoSource("switch.go", dispositionThenBreakSwitchSrc), "API-062")
}

// Kept/Skipped on a type that is not an evo Task, in a file that imports
// evo, is someone else's method.
const nonTaskDispositionSrc = `package p

import evo "github.com/zachbornheimer/evident-output"

type tally struct{ kept, skipped int }

func (t *tally) Kept(n int)    { t.kept += n }
func (t *tally) Skipped(n int) { t.skipped += n }

func count(out *evo.Output, repos []repo) {
	var stats tally
	for _, r := range repos {
		stats.Kept(r.Weight)
	}
	stats.Skipped(1)
	stats.Skipped(2)
	_ = out.Task("count")
}
`

func TestAPI062_NonTaskReceiver_Silent(t *testing.T) {
	assertNoFinding(t, review.GoSource("tally.go", nonTaskDispositionSrc), "API-062")
}
