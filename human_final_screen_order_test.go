package evo

import (
	"context"
	"io"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/testkit"
)

// timingSuffix is the elapsed time a row may end in.
var timingSuffix = regexp.MustCompile(`\s+—?\s*\d+(\.\d+)?(ms|s|m)$`)

// orderedRows is the order a run's rows reach a reader: one entry per row,
// indented by depth, as "<depth><name>". It keeps the row's name and drops
// its glyph, detail and timing suffix, and stops at the conclusion band.
// Plain streams a container that outlasts its grace as a header (`◐ name`)
// and later a closing verdict row (`✓ name`); the Human final screen shows
// that container once, at the position of its first row, so a name repeated
// at the same depth after its first appearance is folded into the first.
func orderedRows(text string) []string {
	var rows []string
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(line, "[") || strings.TrimSpace(line) == "" {
			if len(rows) > 0 && strings.HasPrefix(line, "[") {
				break
			}
			continue
		}
		depth := (len(line) - len(strings.TrimLeft(line, " "))) / 3
		fields := strings.SplitN(strings.TrimSpace(timingSuffix.ReplaceAllString(line, "")), " ", 2)
		if len(fields) < 2 {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimSpace(fields[1]), "  ")
		row := string(rune('0'+depth)) + name
		if depth == 0 && slices.Contains(rows, row) {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

// declareDependents is one program for both renderers: two containers, the second
// After the first, and a Task After both, so every order is fixed by the graph.
func declareDependents(out *Run) {
	g := out.Group("g")
	g.Task("g1").Define(func(context.Context) error { return nil })
	g.Task("g2").Define(func(context.Context) error { return nil })
	s := out.Sequence("s").After(g)
	s.Task("s1").Define(func(context.Context) error { return nil })
	out.Task("afterBoth").After(g, s).Define(func(context.Context) error { return nil })
}

// declareUnrelated is one program for both renderers: two containers that
// share no edge, declared first then second, where first cannot finish until
// second has. Declaration order and settle order differ, and only settle
// order is what Plain writes.
func declareUnrelated(out *Run) {
	secondDone := make(chan struct{})
	first := out.Group("first")
	first.Task("f1").Define(func(context.Context) error { <-secondDone; return nil })
	second := out.Group("second")
	second.Task("s1").Define(func(context.Context) error { close(secondDone); return nil })
}

// TestHumanFinalScreenOrderMatchesPlain pins that the Human final screen
// lists rows in the order Plain writes them: a container before the Task that
// ran After it, and unrelated containers in the order they settled. It
// compares the ordered rows (see orderedRows for how Plain's streamed
// `◐ name` ... `✓ name` block normalizes to the single container row Human
// shows, and which suffixes are stripped), not the set of rows.
func TestHumanFinalScreenOrderMatchesPlain(t *testing.T) {
	for _, c := range []struct {
		name          string
		declare       func(*Run)
		before, after string
	}{
		{"dependents", declareDependents, "0g", "0afterBoth"},
		{"unrelated", declareUnrelated, "0second", "0first"},
	} {
		t.Run(c.name, func(t *testing.T) {
			plain, _ := runPlain(t, Config{}, c.declare)

			screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(24), testkit.NoColor())
			out := initRun(Config{Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Clock: testkit.NewClock(), Terminal: screen}},
				tuning{visibilityDelay: DelayForTest(0), color: colorNever, maxFrameRate: 1_000_000})
			c.declare(out)
			_ = out.close()

			want, got := orderedRows(plain), orderedRows(screen.PersistedText())
			if !slices.Equal(got, want) {
				t.Fatalf("Human final screen rows are ordered %q, Plain writes %q\n--- human ---\n%s\n--- plain ---\n%s",
					got, want, screen.PersistedText(), plain)
			}
			if i, j := slices.Index(got, c.before), slices.Index(got, c.after); i < 0 || j < i {
				t.Fatalf("%q is not above %q: %q", c.before, c.after, got)
			}
		})
	}
}
