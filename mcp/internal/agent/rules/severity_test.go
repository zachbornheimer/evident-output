package rules_test

import (
	"encoding/json"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/rules"
)

// TestEveryRuleHasAValidSeverity keeps the catalog the single, complete
// source of rule severity: review findings take their severity from here.
func TestEveryRuleHasAValidSeverity(t *testing.T) {
	for _, r := range rules.All() {
		if !r.Severity.Valid() {
			t.Errorf("%s: severity %d is not in the closed set", r.ID, r.Severity)
		}
		if sev, ok := rules.SeverityOf(r.ID); !ok || sev != r.Severity {
			t.Errorf("SeverityOf(%s) = %v, %v; want %v", r.ID, sev, ok, r.Severity)
		}
	}
}

// TestSeverity_JSONSpellingIsUnchanged pins the wire form: a Severity
// marshals as the same string the catalog JSON always carried.
func TestSeverity_JSONSpellingIsUnchanged(t *testing.T) {
	for sev, want := range map[rules.Severity]string{
		rules.SeverityError: `"error"`, rules.SeverityWarning: `"warning"`, rules.SeveritySuggestion: `"suggestion"`,
	} {
		got, err := json.Marshal(sev)
		if err != nil || string(got) != want {
			t.Errorf("json.Marshal(%v) = %s, %v; want %s", sev, got, err, want)
		}
		var back rules.Severity
		if err := json.Unmarshal(got, &back); err != nil || back != sev {
			t.Errorf("round trip %s = %v, %v", got, back, err)
		}
	}
	if _, err := json.Marshal(rules.Severity(0)); err == nil {
		t.Error("the zero Severity must not marshal")
	}
}
