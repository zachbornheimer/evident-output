package plain

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render"
)

// TestWriteRunAnnotations covers the plain-mode rendering of run-scoped
// warnings and facts: warnings first, each a "! <text>" line, then facts as
// dim "name value" lines, in call order within each severity.
func TestWriteRunAnnotations(t *testing.T) {
	warnings := []core.Problem{{Summary: "disk nearly full"}}
	facts := []core.Fact{{Name: "branch", Value: "main"}}

	var b strings.Builder
	writeRunAnnotations(&b, warnings, facts, render.Style{Color: false})
	got := b.String()

	if !strings.Contains(got, "disk nearly full") {
		t.Errorf("writeRunAnnotations(...) = %q, want it to contain the warning summary", got)
	}
	if !strings.Contains(got, "branch") || !strings.Contains(got, "main") {
		t.Errorf("writeRunAnnotations(...) = %q, want it to contain the fact name and value", got)
	}
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("writeRunAnnotations(...) wrote %d lines, want 2 (one warning, one fact)", len(lines))
	}
	if !strings.Contains(lines[0], "disk nearly full") {
		t.Errorf("writeRunAnnotations(...) line 0 = %q, want the warning to render before the fact", lines[0])
	}
}

// TestWriteRunAnnotationsEmpty covers the no-op case: nothing to annotate
// writes nothing.
func TestWriteRunAnnotationsEmpty(t *testing.T) {
	var b strings.Builder
	writeRunAnnotations(&b, nil, nil, render.Style{})
	if got := b.String(); got != "" {
		t.Errorf("writeRunAnnotations(nil, nil, ...) = %q, want empty", got)
	}
}
