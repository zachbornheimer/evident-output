package rules

import "testing"

// TestEveryRuleUsesClosedMetadata fails when a rule states a Certainty or
// Detection outside the declared set (an untyped literal still compiles).
func TestEveryRuleUsesClosedMetadata(t *testing.T) {
	for _, r := range All() {
		if !r.Certainty.Valid() {
			t.Errorf("%s: Certainty %q is not a declared Certainty", r.ID, r.Certainty)
		}
		if !r.Detection.Valid() {
			t.Errorf("%s: Detection %q is not a declared Detection", r.ID, r.Detection)
		}
	}
}
