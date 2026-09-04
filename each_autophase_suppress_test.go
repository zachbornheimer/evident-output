package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestEach_BodyPhaseOverride_SuppressesBareItemNameLine is beginner-10:
// Each's bare item-name phase is a courtesy default, not a declared phase.
// When the loop body sets its own Phase before the next paint, only the
// body's phase text should stream as a durable line in plain mode — not
// both the raw item name and the body's replacement.
func TestEach_BodyPhaseOverride_SuppressesBareItemNameLine(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	task := out.Task("install")
	for pkg := range task.Each([]string{"react"}) {
		task.Phase("installing " + pkg + " (resolving deps)")
	}
	task.Done()
	_ = out.Finish()

	rendered := buf.String()
	for _, line := range strings.Split(rendered, "\n") {
		if strings.HasSuffix(strings.TrimSpace(line), "react") {
			t.Fatalf("bare item-name phase line was not suppressed:\n%s", rendered)
		}
	}
	if !strings.Contains(rendered, "installing react (resolving deps)") {
		t.Fatalf("expected the body's own phase text, got:\n%s", rendered)
	}
}
