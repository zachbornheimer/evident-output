package rules_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/agent/rules"
)

func TestMCP027_ExplainFullPayload(t *testing.T) {
	r, ok := rules.Explain("DOM-011")
	if !ok {
		t.Fatal("DOM-011 missing")
	}
	if r.Invariant == "" || r.Why == "" {
		t.Fatalf("missing invariant/why: %+v", r)
	}
	if r.BadCode == "" || r.GoodCode == "" {
		t.Fatalf("missing code examples: %+v", r)
	}
	if r.Remediation == "" {
		t.Fatal("missing remediation")
	}
	if len(r.VerificationIDs) == 0 {
		t.Fatal("missing verification_ids")
	}
	if r.Since == "" {
		t.Fatal("missing since version")
	}
	// Related guidance should point somewhere useful.
	if len(r.RelatedGuidance) == 0 {
		t.Fatal("missing related_guidance")
	}
}

func TestMCP027_ExplainAPI006Examples(t *testing.T) {
	r, ok := rules.Explain("API-006")
	if !ok {
		t.Fatal("API-006")
	}
	if r.BadCode == "" || r.GoodCode == "" || r.Why == "" {
		t.Fatalf("%+v", r)
	}
	for _, id := range r.VerificationIDs {
		if id == "MCP-012" || id == "API-006" {
			return
		}
	}
	t.Fatalf("verification_ids missing MCP-012/API-006: %v", r.VerificationIDs)
}

func TestExplainFirstPaintRules(t *testing.T) {
	for _, id := range []string{"FP-001", "FP-002", "FP-003"} {
		r, ok := rules.Explain(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		if r.Invariant == "" || r.Why == "" || r.BadCode == "" || r.GoodCode == "" || r.Remediation == "" {
			t.Fatalf("%s incomplete payload: %+v", id, r)
		}
		if len(r.RelatedGuidance) == 0 {
			t.Fatalf("%s missing related_guidance", id)
		}
	}
}

func TestMCP028_RuleStabilityVersionPolicy(t *testing.T) {
	ids := rules.IDs()
	if len(ids) < 5 {
		t.Fatal(ids)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate rule id %s", id)
		}
		seen[id] = true
		r, ok := rules.Explain(id)
		if !ok {
			t.Fatal(id)
		}
		if r.Since == "" {
			t.Fatalf("%s missing Since", id)
		}
		// Deprecated rules must name a replacement (policy).
		if r.Deprecated && r.Replacement == "" {
			t.Fatalf("%s deprecated without replacement", id)
		}
	}
	// Stable core IDs required by agent loop docs.
	for _, need := range []string{"API-006", "STREAM-003", "DOM-011", "MCP-021"} {
		if !seen[need] {
			t.Fatalf("missing stable id %s", need)
		}
	}
}
