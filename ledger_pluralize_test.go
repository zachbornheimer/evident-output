package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestLedger_PluralizesSingularObjectFromQuantity is I4: mutation verbs
// take a singular object; the ledger derives the correct plural at render
// time from the quantity — a call site never hand-composes its own
// singular/plural noun or calls evo.Pluralize itself.
func TestLedger_PluralizesSingularObjectFromQuantity(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Task("cleanup").Delete(8, "stale local branch")
	_ = out.Finish()

	rendered := buf.String()
	if !strings.Contains(rendered, "8 stale local branches") {
		t.Fatalf("expected the ledger to pluralize the singular object, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "brancheses") {
		t.Fatalf("double-pluralized, got:\n%s", rendered)
	}
}

// TestLedger_QuantityOne_StaysSingular proves quantity 1 renders the
// singular form unchanged.
func TestLedger_QuantityOne_StaysSingular(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Task("cleanup").Delete(1, "stale local branch")
	_ = out.Finish()

	rendered := buf.String()
	if !strings.Contains(rendered, "1 stale local branch\n") {
		t.Fatalf("expected the singular form at quantity 1, got:\n%s", rendered)
	}
}
