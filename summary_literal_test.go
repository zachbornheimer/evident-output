package evo

import (
	"bytes"
	"strings"
	"testing"
)

// TestSummary_LiteralPercentSurvives is C6 carried to Summary, the 1.1
// replacement for Done(text): Summary takes one string and never runs it
// through fmt.Sprintf, so a literal "%" (e.g. "50% cached") survives.
func TestSummary_LiteralPercentSurvives(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})

	succeed(out.Task("cache"), "50% cached")
	_ = out.finish()

	if !strings.Contains(buf.String(), "50% cached") {
		t.Fatalf("expected the literal percent to survive, got:\n%s", buf.String())
	}
}
