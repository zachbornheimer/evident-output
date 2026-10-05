package review

import (
	"slices"

	"github.com/zachbornheimer/evident-output/internal/agent/rules"
)

// finalize is the one exit every Result's findings pass through: duplicates
// drop, and each finding takes its severity and RequiredVersion from the
// rules catalog, the single place a rule states them. A detector sets
// Severity itself only for a documented variant that deliberately differs
// from its rule's catalog severity (EVO-FILE-001's freshness boundary).
func finalize(fs []Finding) []Finding {
	out := dedupe(fs)
	for i := range out {
		if minDialect, ok := rules.MinDialectOf(out[i].RuleID); ok {
			out[i].RequiredVersion = minDialect
		}
		if out[i].Severity != "" {
			continue
		}
		if sev, ok := rules.SeverityOf(out[i].RuleID); ok {
			out[i].Severity = sev.String()
		}
	}
	return out
}

// admitDialect drops every finding whose rule needs a newer release than
// desiredVersion: its suggestion names API that pin cannot call.
func admitDialect(fs []Finding, desiredVersion string) []Finding {
	return slices.DeleteFunc(fs, func(f Finding) bool {
		minDialect, _ := rules.MinDialectOf(f.RuleID)
		return minDialect != "" && !dialectAtLeast(desiredVersion, minDialect)
	})
}

// newResult is a Result over finalized findings, with RecheckRequired
// derived from them.
func newResult(fs []Finding) Result {
	out := finalize(fs)
	return Result{Findings: out, RecheckRequired: hasRequired(out)}
}
