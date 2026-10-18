package evo

import (
	"bytes"
	"strings"
	"testing"
)

// TestOutput_Subject_PostInitSetter is I3: a caller who doesn't know the
// subject text until after Init can call out.SubjectForTest(text) instead of
// Config.Subject — same one-shot durable-line semantics.
func TestOutput_Subject_PostInitSetter(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})

	out.subjectForTest("/repos/bpp-csharp")

	if !strings.Contains(buf.String(), "/repos/bpp-csharp") {
		t.Fatalf("expected the subject line, got:\n%s", buf.String())
	}
}
