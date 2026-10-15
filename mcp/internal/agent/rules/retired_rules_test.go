package rules

import "testing"

// A retired ID is gone from the live catalog for good: explain answers it
// from RetiredRules, and nothing may register a live rule under it again.
func TestRetiredRulesAreNotLive(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range RetiredRules() {
		if seen[r.ID] {
			t.Errorf("retired rule %s listed twice", r.ID)
		}
		seen[r.ID] = true
		if _, live := Explain(r.ID); live {
			t.Errorf("rule %s is retired and still in the live catalog", r.ID)
		}
		if r.RetiredIn == "" || r.Why == "" || r.Instead == "" {
			t.Errorf("retired rule %s needs RetiredIn, Why and Instead", r.ID)
		}
	}
}
