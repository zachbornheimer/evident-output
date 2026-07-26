// Package rules is the stable review-rule registry (Appendix C namespaces).
package rules

// Rule is one stable diagnostic rule.
type Rule struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Invariant   string `json:"invariant"`
	Remediation string `json:"remediation"`
}

// All returns the v1 rule registry subset.
func All() []Rule {
	return []Rule{
		{ID: "API-006", Category: "API", Severity: "warning", Invariant: "explicit Start is optional", Remediation: "Use Phase/Progress or direct Done/OK"},
		{ID: "API-027", Category: "API", Severity: "error", Invariant: "Task cannot contain children; Tasks has no leaf lifecycle", Remediation: "Use Tasks.Task for children"},
		{ID: "STREAM-003", Category: "STREAM", Severity: "error", Invariant: "progress must not contaminate structured stdout", Remediation: "Route UI to diagnostic stream"},
		{ID: "SEC-001", Category: "SEC", Severity: "error", Invariant: "untrusted text cannot control the terminal", Remediation: "Sanitize caller text; use Detail/Cause split"},
		{ID: "DOM-011", Category: "DOM", Severity: "error", Invariant: "first terminal state wins", Remediation: "Resolve only after final state is known"},
		{ID: "TERM-001", Category: "TERM", Severity: "warning", Invariant: "instant completion does not flash spinner", Remediation: "Rely on visibility delay"},
		{ID: "MCP-021", Category: "MCP", Severity: "error", Invariant: "agents stop only when recheck_required is false", Remediation: "Loop review until clean"},
	}
}

// Explain returns a rule by ID.
func Explain(id string) (Rule, bool) {
	for _, r := range All() {
		if r.ID == id {
			return r, true
		}
	}
	return Rule{}, false
}
