package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestFail_NoDetail_DropsRedundantProblemRow is beginner-3: a Fail/Block
// call with no Detail beyond its own summary must not re-echo that summary
// as a "└─ <same text>" row underneath the glyph line — that told the reader
// nothing they didn't already see.
func TestFail_NoDetail_DropsRedundantProblemRow(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Task("build").Fail("compile failed")
	_ = out.Finish()

	rendered := buf.String()
	if strings.Count(rendered, "compile failed") != 1 {
		t.Fatalf("expected \"compile failed\" to appear exactly once (no re-echoed problem row), got:\n%s", rendered)
	}
	if strings.Contains(rendered, "└─") {
		t.Fatalf("expected no problem row at all, got:\n%s", rendered)
	}
}

// TestWarn_MessagePlacement_MatchesFailBlock is beginner-3: Warn's message
// gets the same summary placement as Fail/Block (the task's own glyph row
// carries it), with the same de-echo dropping the redundant problem row.
func TestWarn_MessagePlacement_MatchesFailBlock(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Task("optional tool").Warn("shellcheck not found")
	_ = out.Finish()

	rendered := buf.String()
	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	if len(lines) == 0 || !strings.Contains(lines[0], "optional tool") || !strings.Contains(lines[0], "shellcheck not found") {
		t.Fatalf("expected the warning message on the same glyph row as the task name (Fail/Block placement), got:\n%s", rendered)
	}
	if strings.Count(rendered, "shellcheck not found") != 1 {
		t.Fatalf("expected the message to appear exactly once, got:\n%s", rendered)
	}
}
