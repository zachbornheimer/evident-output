package review

import "github.com/zachbornheimer/evident-output/internal/agent/rules"

// finalize is the one exit every Result's findings pass through: duplicates
// drop, and each finding takes its severity from the rules catalog, the
// single place a rule's severity is stated. A detector sets Severity itself
// only for a documented variant that deliberately differs from its rule's
// catalog severity (EVO-FILE-001's freshness boundary).
func finalize(fs []Finding) []Finding {
	out := dedupe(fs)
	for i := range out {
		if out[i].Severity != "" {
			continue
		}
		if sev, ok := rules.SeverityOf(out[i].RuleID); ok {
			out[i].Severity = sev.String()
		}
	}
	return out
}

// newResult is a Result over finalized findings, with RecheckRequired
// derived from them.
func newResult(fs []Finding) Result {
	out := finalize(fs)
	return Result{Findings: out, RecheckRequired: hasRequired(out)}
}
