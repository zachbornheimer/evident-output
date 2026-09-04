package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestPlain_FailedTaskSummaryAndEvidenceFullIntensity is release-gate round
// 6 finding 5: plain.go dimmed every task summary, including a Fail/Block
// task's — its own summary and evidence must never be the lowest-contrast
// text on screen. Asserted on rendered SGR bytes: the summary and the
// Detail evidence line render with no dim (\x1b[2m) wrap; only the
// decorative "└─" connector may still be dim.
func TestPlain_FailedTaskSummaryAndEvidenceFullIntensity(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain()}})

	task := out.Task("branches")
	task.Fail("could not delete", evo.Detail("permission denied: origin/main"))

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (misuse-only, not a Failed conclusion)", err)
	}

	got := buf.String()
	const dim = "\x1b[2m"
	if strings.Contains(got, dim+"could not delete") {
		t.Fatalf("Fail summary must render at full intensity, not dim:\n%q", got)
	}
	if strings.Contains(got, dim+"permission denied: origin/main") {
		t.Fatalf("Fail evidence must render at full intensity, not dim:\n%q", got)
	}
	if !strings.Contains(got, "could not delete") || !strings.Contains(got, "permission denied: origin/main") {
		t.Fatalf("expected both summary and evidence text present:\n%q", got)
	}
}
