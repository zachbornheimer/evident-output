package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestOutput_Subject_PostInitSetter is I3: a caller who doesn't know the
// subject text until after Init can call out.Subject(text) instead of
// Config.Subject — same one-shot durable-line semantics.
func TestOutput_Subject_PostInitSetter(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.NoColor()}})

	out.Subject("/repos/bpp-csharp")

	if !strings.Contains(buf.String(), "/repos/bpp-csharp") {
		t.Fatalf("expected the subject line, got:\n%s", buf.String())
	}
}
