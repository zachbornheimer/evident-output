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

// TestToJSONTask_ProjectsVerificationFacts is ZYS-823's public output.v1
// counterpart to wire.toTaskDoc's Verification fix: toJSONTask previously
// had no Verification field at all, so evo.EncodeJSON's public "evo.run"-
// sibling document dropped every File/Patch per-attribute verification
// outcome (and its Facts) a Task recorded, the same machine-truth loss for
// this legacy projection.
func TestToJSONTask_ProjectsVerificationFacts(t *testing.T) {
	in := core.TaskSnapshot{
		ID: "task_1", Name: "write launch agent",
		Verification: []core.VerificationDetail{
			{Name: "contents", Status: core.VerificationSatisfied},
			{
				Name: "permissions", Status: core.VerificationError,
				Facts: []core.Fact{
					{Name: "error", Value: "operation not permitted"},
					{Name: "path", Value: "~/Library/LaunchAgents/com.acme.prod.agent.plist"},
				},
			},
		},
	}

	out := toJSONTask(in)

	if len(out.Verification) != 2 {
		t.Fatalf("Verification = %+v, want 2 entries", out.Verification)
	}
	if out.Verification[0].Name != "contents" || out.Verification[0].Status != string(core.VerificationSatisfied) {
		t.Fatalf("Verification[0] = %+v", out.Verification[0])
	}
	perm := out.Verification[1]
	if perm.Name != "permissions" || perm.Status != string(core.VerificationError) {
		t.Fatalf("Verification[1] = %+v", perm)
	}
	if len(perm.Facts) != 2 || perm.Facts[0].Name != "error" || perm.Facts[0].Value != "operation not permitted" {
		t.Fatalf("Verification[1].Facts = %+v, want the error fact preserved", perm.Facts)
	}
}
