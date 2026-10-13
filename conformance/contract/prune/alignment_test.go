// Package prune_test binds contract §18 and §30 "Plain Effect alignment" to
// the public evo API using the zq prune fixture.
package prune_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

const effectColumnGap = "  "

type ledgerRow struct {
	subject string
	spec    evo.EffectSpec
	// cells is the subject's display width in terminal cells.
	cells int
}

var asciiRows = []ledgerRow{
	{"branches", evo.EffectSpec{Verb: evo.Delete, Object: "local tip", Quantity: 40}, len("branches")},
	{"worktrees", evo.EffectSpec{Verb: evo.Delete, Object: "worktree", Quantity: 1}, len("worktrees")},
	{"remote-tracking", evo.EffectSpec{Verb: evo.Delete, Object: "stale origin/* branch", Quantity: 4}, len("remote-tracking")},
}

func renderLedger(t *testing.T, dryRun bool, rows []ledgerRow) string {
	t.Helper()
	var buf bytes.Buffer
	_ = evo.Execute(context.Background(), evo.Config{DryRun: dryRun, Name: "zq", Output: evo.Plain, Facades: evo.Facades{Stdout: &buf, Stderr: &buf}}, func(ctx context.Context, r *evo.Run) error {
		for _, row := range rows {
			task := r.Task(row.subject)
			task.Define(func(ctx context.Context) error {
				return evo.Effect(ctx, row.spec, func(context.Context) error { return nil })
			})
		}
		return nil
	})
	return buf.String()
}

func requireAligned(t *testing.T, rendered, tag string, rows []ledgerRow) {
	t.Helper()
	widest := 0
	for _, row := range rows {
		widest = max(widest, row.cells)
	}
	lines := strings.Split(rendered, "\n")
	for _, row := range rows {
		want := tag + row.subject + strings.Repeat(" ", widest-row.cells) + effectColumnGap
		found := false
		for _, line := range lines {
			if !strings.HasPrefix(line, tag+row.subject) {
				continue
			}
			found = true
			rest, ok := strings.CutPrefix(line, want)
			if !ok || strings.HasPrefix(rest, " ") {
				t.Errorf("row %q = %q, want prefix %q then the Effect phrase; full output:\n%s", row.subject, line, want, rendered)
			}
		}
		if !found {
			t.Errorf("no %q row for %q in:\n%s", tag, row.subject, rendered)
		}
	}
}

func TestC30_058_SubjectsPadToTheWidestSubjectThenTwoSpaces(t *testing.T) {
	requireAligned(t, renderLedger(t, true, asciiRows), "[planned] ", asciiRows)
}

func TestC30_059_PlannedAndChangedBandsUseTheSameAlignment(t *testing.T) {
	requireAligned(t, renderLedger(t, true, asciiRows), "[planned] ", asciiRows)
	requireAligned(t, renderLedger(t, false, asciiRows), "[changed] ", asciiRows)
}
