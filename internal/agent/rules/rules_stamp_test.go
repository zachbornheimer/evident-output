package rules_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/rules"
)

func TestExplainEvoStampFactEffectFamily(t *testing.T) {
	for _, id := range []string{
		"EVO-STAMP-001", "EVO-STAMP-002", "EVO-FACT-001", "EVO-EFFECT-001",
	} {
		r, ok := rules.Explain(id)
		if !ok {
			t.Fatalf("%s missing from registry", id)
		}
		if r.Invariant == "" || r.Why == "" || r.BadCode == "" || r.GoodCode == "" || r.Remediation == "" {
			t.Fatalf("%s incomplete payload: %+v", id, r)
		}
		if r.Since != "1.0.0" {
			t.Fatalf("%s must be Since 1.0.0, got %q", id, r.Since)
		}
		if r.Detection == "guidance" {
			t.Fatalf("%s must have a live detector, not Detection=guidance", id)
		}
		if len(r.VerificationIDs) == 0 {
			t.Fatalf("%s missing verification_ids", id)
		}
	}
}
