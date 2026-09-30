//go:build evopending

// Package prune_test holds contract §30 "Plain Effect alignment" behavior
// that the plain renderer does not meet yet. It fails today because the plain
// ledger measures the subject column in runes (engine maxSubjectWidth), not
// display cells.
package prune_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

const effectColumnGap = "  "

type wideRow struct {
	subject string
	// cells is the subject's width in terminal cells: each CJK rune is two.
	cells int
	spec  evo.EffectSpec
}

var wideRows = []wideRow{
	{"分支", 4, evo.EffectSpec{Verb: evo.EffectDelete, Object: "local tip", Quantity: 2}},
	{"ab", 2, evo.EffectSpec{Verb: evo.EffectRemove, Object: "worktree", Quantity: 1}},
}

func TestC30_058_SubjectsPadToTheWidestDisplayCellWidth(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Color: evo.ColorNever, DryRun: true,
		Title: "zq", Subject: "zq prune  ~/repo", Stdout: &buf, Stderr: &buf,
	})
	t.Cleanup(func() { _ = out.Close() })
	for _, row := range wideRows {
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

	const widest = 4
	for _, row := range wideRows {
		want := "[planned] " + row.subject + strings.Repeat(" ", widest-row.cells) + effectColumnGap
		rest, found := "", false
		for line := range strings.SplitSeq(buf.String(), "\n") {
			if strings.HasPrefix(line, "[planned] "+row.subject) {
				rest, found = strings.CutPrefix(line, want)
				break
			}
		}
		if !found || strings.HasPrefix(rest, " ") {
			t.Errorf("row %q is not padded to the widest cell width; want prefix %q in:\n%s", row.subject, want, buf.String())
		}
	}
}
