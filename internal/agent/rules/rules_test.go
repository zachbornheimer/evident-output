package rules_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/rules"
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

func TestExplainLAYOUT001AndLAYOUT002(t *testing.T) {
	for _, id := range []string{"LAYOUT-001", "LAYOUT-002"} {
		r, ok := rules.Explain(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		if r.Invariant == "" || r.Why == "" || r.BadCode == "" || r.GoodCode == "" || r.Remediation == "" {
			t.Fatalf("%s incomplete payload: %+v", id, r)
		}
		if r.Detection == "guidance" {
			t.Fatalf("%s must have a live detector, not Detection=guidance", id)
		}
	}
	dual, _ := rules.Explain("LAYOUT-001")
	if !strings.Contains(dual.BadCode, "clean_repo") && !strings.Contains(dual.BadCode, "cleanRepo") {
		t.Fatalf("LAYOUT-001 BadCode must show leftover clean_repo/cleanRepo naming, got %q", dual.BadCode)
	}
	if !strings.Contains(dual.GoodCode, "Aliases") || !strings.Contains(dual.GoodCode, "clean-repo") {
		t.Fatalf("LAYOUT-001 GoodCode must show Aliases clean-repo on prune, got %q", dual.GoodCode)
	}
	folder, _ := rules.Explain("LAYOUT-002")
	if !strings.Contains(folder.BadCode, "internal/app") {
		t.Fatalf("LAYOUT-002 BadCode must show internal/app, got %q", folder.BadCode)
	}
	if !strings.Contains(folder.GoodCode, "internal/purge") && !strings.Contains(folder.GoodCode, "purge.Command") {
		t.Fatalf("LAYOUT-002 GoodCode must own RunE under internal/purge or thin-delegate Command(), got %q", folder.GoodCode)
	}
}

func TestExplainCALL001(t *testing.T) {
	r, ok := rules.Explain("CALL-001")
	if !ok {
		t.Fatal("CALL-001 missing")
	}
	if r.Invariant == "" || r.Why == "" || r.BadCode == "" || r.GoodCode == "" || r.Remediation == "" {
		t.Fatalf("CALL-001 incomplete payload: %+v", r)
	}
	if !strings.Contains(r.BadCode, "make(") {
		t.Fatalf("CALL-001 BadCode must show inline make, got %q", r.BadCode)
	}
	if strings.Contains(r.GoodCode, "make([]evo.Fact, 0)}") || strings.Contains(r.GoodCode, "Facts: make(") {
		t.Fatalf("CALL-001 GoodCode must extract make to a named local, got %q", r.GoodCode)
	}
}

func TestExplainFirstPaintRules(t *testing.T) {
	for _, id := range []string{"FP-001", "FP-002", "FP-003", "FP-005"} {
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

// TestGuidanceOnlyRulesAreMarked proves CON-001 and GLYPH-001 explicitly
// declare Detection="guidance" — no cheap honest static detector exists for
// either, and the rule payload must say so rather than leave an agent
// waiting on a review finding that never arrives.
func TestGuidanceOnlyRulesAreMarked(t *testing.T) {
	for _, id := range []string{"CON-001", "GLYPH-001"} {
		r, ok := rules.Explain(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		if r.Detection != "guidance" {
			t.Fatalf("%s want Detection=guidance, got %q", id, r.Detection)
		}
	}
}

func TestAPI032_MutationCallbackIsGoodCode(t *testing.T) {
	r, ok := rules.Explain("API-032")
	if !ok {
		t.Fatal("API-032 missing")
	}
	if !strings.Contains(r.BadCode, `Delete(n, "local tip")`) && !strings.Contains(r.BadCode, "Delete(n,") {
		t.Fatalf("API-032 BadCode must show positional quantity-first Delete, got %q", r.BadCode)
	}
	if strings.Contains(r.GoodCode, `Delete(n, "local tip")`) {
		t.Fatalf("API-032 GoodCode must not teach quantity-first Delete, got %q", r.GoodCode)
	}
	hasObjectFirst := strings.Contains(r.GoodCode, `Delete("`)
	hasCallback := strings.Contains(r.GoodCode, "func()") || strings.Contains(r.GoodCode, "Affected")
	if !hasObjectFirst || !hasCallback {
		t.Fatalf("API-032 GoodCode must teach Delete(object, fn)/Affected, got %q", r.GoodCode)
	}
}

func TestAPI026_DoesNotBanEvoScheduler(t *testing.T) {
	r, ok := rules.Explain("API-026")
	if !ok {
		t.Fatal("API-026 missing")
	}
	for _, blob := range []string{r.Invariant, r.Why, r.GoodCode, r.Remediation} {
		if strings.Contains(blob, "must not grow schedulers") {
			t.Fatalf("API-026 must not ban Evo's scheduler: %q", blob)
		}
	}
	if !strings.Contains(r.GoodCode, "Group") || !strings.Contains(r.GoodCode, "Task") || !strings.Contains(r.GoodCode, "Define") {
		t.Fatalf("API-026 GoodCode must teach Group.Task+Define, got %q", r.GoodCode)
	}
	if !strings.Contains(r.BadCode, "Map") && !strings.Contains(r.BadCode, "Retry") {
		t.Fatalf("API-026 BadCode must still show Map/Retry, got %q", r.BadCode)
	}
}

func TestAPI027_TeachingNamesGroup(t *testing.T) {
	r, ok := rules.Explain("API-027")
	if !ok {
		t.Fatal("API-027 missing")
	}
	retired := "Display" + "Group"
	if strings.Contains(r.Invariant, retired) || strings.Contains(r.Remediation, retired) {
		t.Fatalf("API-027 teaching must name Group, not the retired constructor: %+v", r)
	}
	if !strings.Contains(r.GoodCode, "Group") {
		t.Fatalf("API-027 GoodCode must name Group, got %q", r.GoodCode)
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
