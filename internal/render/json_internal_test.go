package render

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// TestToJSONProblems_PreservesEvidenceTail is ZYS-823 gap 2: a Problem's
// EvidenceTail is machine truth a plain/TTY reader can see (writeProblemDetail
// renders it alongside or in place of Detail). Before this fix,
// toJSONProblems silently dropped it — the final JSON projection lost the
// evidence entirely, a stronger loss than mere truncation.
func TestToJSONProblems_PreservesEvidenceTail(t *testing.T) {
	tail := strings.Repeat("evidence line that would clip in a narrow TTY. ", 20)
	in := []core.Problem{{
		Summary:      "build failed",
		Detail:       "compiler error",
		EvidenceTail: tail,
	}}

	out := toJSONProblems(in)

	if len(out) != 1 {
		t.Fatalf("toJSONProblems returned %d problems, want 1", len(out))
	}
	if out[0].EvidenceTail != tail {
		t.Fatalf("EvidenceTail = %q, want the full untruncated tail %q", out[0].EvidenceTail, tail)
	}
	if out[0].Detail != "compiler error" {
		t.Fatalf("Detail = %q, want it to still render alongside EvidenceTail", out[0].Detail)
	}
}
