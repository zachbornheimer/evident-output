package rules_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/rules"
)

// TestExplainEvoUIWireFamily covers the spec §57 EVO-UI-*/EVO-WIRE-*/
// EVO-EXIT-*/EVO-LIVE-* rules: every one is registered, has a full payload,
// and none is Detection=guidance: EVO-UI-004 and EVO-WIRE-002 gained
// detectors (C21-012, C21-014) and must stay detectable.
func TestExplainEvoUIWireFamily(t *testing.T) {
	guidanceOnly := map[string]bool{}
	for _, id := range []string{
		"EVO-UI-001", "EVO-UI-002", "EVO-UI-003", "EVO-UI-004",
		"EVO-WIRE-001", "EVO-WIRE-002", "EVO-WIRE-003",
		"EVO-EXIT-001", "EVO-LIVE-001",
	} {
		r, ok := rules.Explain(id)
		if !ok {
			t.Fatalf("%s missing from registry", id)
		}
		if r.Invariant == "" || r.Why == "" || r.BadCode == "" || r.GoodCode == "" || r.Remediation == "" {
			t.Fatalf("%s incomplete payload: %+v", id, r)
		}
		if len(r.VerificationIDs) == 0 {
			t.Fatalf("%s missing verification_ids", id)
		}
		if r.Since != "1.0.0" {
			t.Fatalf("%s must be Since 1.0.0, got %q", id, r.Since)
		}
		wantGuidance := guidanceOnly[id]
		if gotGuidance := r.Detection == "guidance"; gotGuidance != wantGuidance {
			t.Fatalf("%s Detection=%q, want guidance=%v", id, r.Detection, wantGuidance)
		}
	}
}
