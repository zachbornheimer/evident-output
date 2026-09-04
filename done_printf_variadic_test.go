package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestDone_LiteralPercentSurvivesWithoutArgs is C6: Done's one-string-arg
// call never runs through fmt.Sprintf — a literal "%" in a caller's summary
// (e.g. "50% cached") must survive untouched, exactly like Task's own
// "no args leaves name untouched" rule.
func TestDone_LiteralPercentSurvivesWithoutArgs(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Task("cache").Done("50% cached")
	_ = out.Finish()

	if !strings.Contains(buf.String(), "50% cached") {
		t.Fatalf("expected the literal percent to survive, got:\n%s", buf.String())
	}
}

// TestDone_FormatsWithArgs is C6: Done applies fmt.Sprintf when args follow
// the first string, one text spelling shared with Task/Group/Reason/Warn —
// no separate Donef.
func TestDone_FormatsWithArgs(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Task("install").Done("%d packages", 18)
	_ = out.Finish()

	if !strings.Contains(buf.String(), "18 packages") {
		t.Fatalf("expected the formatted summary, got:\n%s", buf.String())
	}
}

// TestDone_NoArgs_NoSummary is C6: Done() with no arguments at all still
// works exactly like the old zero-arg convenience call.
func TestDone_NoArgs_NoSummary(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Task("build").Done()
	_ = out.Finish()

	if !strings.Contains(buf.String(), "build") {
		t.Fatalf("expected the bare Done row, got:\n%s", buf.String())
	}
}
