package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestDeclareDryRun_BeforeAnyRow_SwitchesMode is I8: a caller who doesn't
// know DryRun until after Init (e.g. resolved from a flag) can call
// out.DeclareDryRun() before any other call, and mutation verbs render
// [planned] exactly as if Config.DryRun had been set at construction.
func TestDeclareDryRun_BeforeAnyRow_SwitchesMode(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.DeclareDryRun()
	out.Task("cleanup").Delete(2, "stale local branch")
	_ = out.Finish()

	rendered := buf.String()
	if !strings.Contains(rendered, "[dry-run]") {
		t.Fatalf("expected the dry-run marker, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "[planned]") {
		t.Fatalf("expected a [planned] row, got:\n%s", rendered)
	}
}

// TestDeclareDryRun_AfterADurableRow_IsMisuse is I8's bound: calling
// DeclareDryRun after a durable row has already streamed is misuse — the
// earlier row cannot retroactively reflect the switch.
func TestDeclareDryRun_AfterADurableRow_IsMisuse(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Println("Reading configuration")
	out.DeclareDryRun()

	if err := out.Finish(); err == nil {
		t.Fatal("Finish() = nil, want the recorded misuse")
	}
}
