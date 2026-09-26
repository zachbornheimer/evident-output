package rules

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/retired"
)

func TestUnexplainedInAllowsARemovalNote(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"use task.Record(verb, n, object) when the work already happened", 1},
		{"Record was removed in 1.1; task.Record(...) becomes evo.Effect", 0},
		{"range Group.Each so every iteration is visible", 1},
		{"Group.Each was removed in 1.0", 0},
		{"Group.Each was removed in 1.1", 1},
		{"Doing/Progress/Done already activate the task", 1},
		{"select { case <-ctx.Done(): }", 0},
		{"[]evo.Option is superseded; use Config fields", 1},
		{"[]evo.Option is superseded for this pin, and removed in 1.1; use Config fields", 0},
	}
	for _, c := range cases {
		if got := len(UnexplainedIn(c.text)); got != c.want {
			t.Errorf("UnexplainedIn(%q) = %d hits, want %d", c.text, got, c.want)
		}
	}
}

func TestEverySymbolHasAReleaseAndReplacement(t *testing.T) {
	for _, s := range retired.Symbols() {
		if s.Replacement == "" || s.RemovedIn == "" {
			t.Errorf("%s: Replacement and RemovedIn are required", s.Contract)
		}
	}
}

// TestTaughtInCoversEveryOwnerFreezeSymbol pins each of the owner
// vocabulary freeze's retired names (ZYS-950/812, the 2026-09-25 freezes)
// to at least one text that its Taught regex must match, so a rewritten or
// loosened pattern shows up as a test failure instead of silently missing
// (or over-matching) the name it exists to catch.
func TestTaughtInCoversEveryOwnerFreezeSymbol(t *testing.T) {
	cases := map[string]string{
		"ID":                 "evo.ID(\"x\")",
		"EntityOption":       "an EntityOption built from Task",
		"StartPhase":         "evo.StartPhase(\"fetching\")",
		"TaskHandle.Failf(":  "task.Failf(\"x: %w\", err)",
		"TaskHandle.Blockf(": "task.Blockf(\"x: %w\", err)",
		"Output.Failf(":      "out.Failf(\"x: %w\", err)",
		"Failure":            "return evo.Failure{Summary: \"x\"}",
		"TaskHandle.Step(":   "task.Step(\"a\")",
		"TaskHandle.Kept(":   "task.Kept(reason)",
		"ForSkip":            "evo.ForSkip(\"reason\")",
		"OnTask":             "evo.OnTask(\"reason\")",
		"ReasonOption":       "a ReasonOption argument",
		"Option":             "[]evo.Option{evo.Plain()}",
	}
	// The canonical per-item idiom chains straight off Task(...), with no
	// bare task-named receiver before the verb (§3.1) — the plain
	// "[Tt]ask\w*\." form alone never sees these, so they get their own
	// cases rather than sharing the table above.
	chainedCases := map[string]string{
		"TaskHandle.Step(":   "out.Task(\"x\").Step(\"a\")",
		"TaskHandle.Kept(":   "group.Task(item).Kept(reason)",
		"TaskHandle.Failf(":  "group.Task(item).Failf(\"x: %w\", err)",
		"TaskHandle.Blockf(": "group.Task(item).Blockf(\"x: %w\", err)",
	}
	for contract, text := range chainedCases {
		found := false
		for _, h := range retired.TaughtIn(text) {
			if h.Symbol.Contract == contract {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("TaughtIn(%q) did not match Contract %q", text, contract)
		}
	}
	for contract, text := range cases {
		found := false
		for _, h := range retired.TaughtIn(text) {
			if h.Symbol.Contract == contract {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("TaughtIn(%q) did not match Contract %q", text, contract)
		}
	}
}
