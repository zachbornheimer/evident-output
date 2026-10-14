package evo_test

import (
	"context"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// stopperEnding is one way the gate Task can end without succeeding: the
// verb it returns, the glyph and state its row shows, and how a Task kept
// from running names it.
type stopperEnding struct {
	name   string
	verb   func() error
	glyph  string
	state  string
	runEnd string
}

var stopperEndings = []stopperEnding{
	{"refuse", func() error { return evo.Refuse("no") }, "⊘", "refused", "[refused · partial]"},
	{"exclude", func() error { return evo.Exclude("no") }, "○", "excluded", "[excluded · partial]"},
	{"fail", func() error { return evo.Fail("no") }, "✗", "failed", "[failed · partial]"},
}

// stopperRun runs gate, then Group work After gate (members from Build when
// built, declared directly otherwise), then root Task report After work.
func stopperRun(t *testing.T, built bool, ending stopperEnding, output evo.Output) contractRun {
	t.Helper()
	ok := func(context.Context) error { return nil }
	return executeContract(context.Background(), t, evo.Config{Name: "stopper", Output: output},
		func(_ context.Context, r *evo.Run) error {
			// Every root name is declared before gate can end, so the
			// name column is as wide on every run.
			gate, report := r.Task("gate"), r.Task("report")
			work := r.Group("work")
			work.After(gate)
			if built {
				work.Build(func(g *evo.Group) { g.Task("w1").Define(ok) })
			} else {
				work.Task("w1").Define(ok)
			}
			report.After(work).Define(ok)
			gate.Define(func(context.Context) error { return ending.verb() })
			return nil
		})
}

// TestNotStartedNamesRealStopperForBuiltGroups pins CONTRACT.md §2 and §8: a
// Task kept from running by a Group names the Task that really stopped it,
// in its real state, whether the Group's members come from Build or were
// declared directly, and a row never prints above the collection that
// explains it. A built Group whose gate never succeeded ran nothing, so the
// Task after it is not "stopped by work failed": work never ran.
func TestNotStartedNamesRealStopperForBuiltGroups(t *testing.T) {
	for _, ending := range stopperEndings {
		t.Run("built/"+ending.name, func(t *testing.T) {
			stopped := "not started — gate " + ending.state
			golden := ending.glyph + " gate    no\n" +
				"- work  " + stopped + "\n" +
				"- report  " + stopped + "\n\n" +
				ending.runEnd + "  stopper\n"
			assertStopperRun(t, true, ending, golden, stopped)
		})
		t.Run("static/"+ending.name, func(t *testing.T) {
			golden := ending.glyph + " gate    no\n" +
				"- work\n" +
				"   - w1  not started — gate " + ending.state + "\n" +
				"- report  not started — w1 not started\n\n" +
				ending.runEnd + "  stopper\n"
			assertStopperRun(t, false, ending, golden, "not started — w1 not started")
		})
	}
}

// assertStopperRun checks the Plain text equals golden and evo.run carries
// reportSummary on report.
func assertStopperRun(t *testing.T, built bool, ending stopperEnding, golden, reportSummary string) {
	t.Helper()
	if got := stopperRun(t, built, ending, evo.Plain).Stderr; got != golden {
		t.Errorf("Plain output =\n%s\nwant\n%s", got, golden)
	}
	doc := runDocument(t, stopperRun(t, built, ending, evo.JSON).Stdout)
	for _, task := range documentTasks(t, doc) {
		if task["name"] == "report" {
			if got, _ := task["summary"].(string); got != reportSummary {
				t.Errorf("evo.run report summary = %q, want %q", got, reportSummary)
			}
			return
		}
	}
	t.Error("evo.run has no report task")
}
