package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestPhaseAndSkip_ArePrintfVariadic is release-gate round 6 finding 4:
// Phase and Skip must share the same no-args/literal/printf-formatted text
// shape as Done/Task/Group/Reason (C6) — Confirm is the one entity-text
// spelling left non-printf.
func TestPhaseAndSkip_ArePrintfVariadic(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})

	phased := out.Task("fetch")
	phased.Phase("resolving %s", "main")
	if got := phased.Snapshot().Phase; got != "resolving main" {
		t.Fatalf("Phase must format its printf args, got phase %q", got)
	}
	phased.Done()

	skipped := out.Task("prune")
	skipped.Skip("not needed on %s", "main")

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil", err)
	}

	got := buf.String()
	if !strings.Contains(got, "not needed on main") {
		t.Fatalf("Skip must format its printf args, got:\n%s", got)
	}
}
