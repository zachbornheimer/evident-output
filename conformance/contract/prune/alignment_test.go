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
	{"branches", evo.EffectSpec{Verb: evo.EffectDelete, Object: "local tip", Quantity: 40}, len("branches")},
	{"worktrees", evo.EffectSpec{Verb: evo.EffectRemove, Object: "worktree", Quantity: 1}, len("worktrees")},
	{"remote-tracking", evo.EffectSpec{Verb: evo.EffectDelete, Object: "stale origin/* branch", Quantity: 4}, len("remote-tracking")},
}

func renderLedger(t *testing.T, dryRun bool, rows []ledgerRow) string {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Color: evo.ColorNever, DryRun: dryRun,
		Title: "zq", Subject: "zq prune  ~/repo", Stdout: &buf, Stderr: &buf,
	})
	t.Cleanup(func() { _ = out.Close() })
	for _, row := range rows {
		task := out.Task(row.subject)
		task.Define(func(ctx context.Context) error {
			return evo.Effect(ctx, row.spec, func(context.Context) error { return nil })
		})
		if err := task.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
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
