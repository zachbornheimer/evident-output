package review

import "github.com/zachbornheimer/evident-output/internal/agent/rules"

// defaultConformanceTargetVersion is the target_version spec §60's example
// output names when a caller does not pin one — the module's next
// unreleased version, which is what "is this Evo usage current" means
// absent an explicit desired_version.
const defaultConformanceTargetVersion = "next"

// ConformanceFinding is one row of the spec §60 MCP conformance shape.
type ConformanceFinding struct {
	Rule      string `json:"rule"`
	Severity  string `json:"severity"`
	File      string `json:"file,omitempty"`
	Line      int    `json:"line,omitempty"`
	Summary   string `json:"summary"`
	Migration string `json:"migration,omitempty"`
}

// ConformanceReport is the spec §60 MCP conformance operation output:
// "is this Evo usage current, what is stale, what is unsafe, what can be
// mechanically upgraded, what provenance is explicit vs opaque, what
// requires developer intent" — answered per finding via its rule.
type ConformanceReport struct {
	TargetVersion string               `json:"target_version"`
	Findings      []ConformanceFinding `json:"findings"`
}

// Conformance reshapes a review Result into the spec §60 conformance shape.
// targetVersion falls back to res.DesiredVersion, then to "next".
func Conformance(res Result, targetVersion string) ConformanceReport {
	target := targetVersion
	if target == "" {
		target = res.DesiredVersion
	}
	if target == "" {
		target = defaultConformanceTargetVersion
	}
	findings := make([]ConformanceFinding, 0, len(res.Findings))
	for _, f := range res.Findings {
		findings = append(findings, ConformanceFinding{
			Rule:      f.RuleID,
			Severity:  f.Severity,
			File:      f.File,
			Line:      f.Line,
			Summary:   f.Message,
			Migration: conformanceMigration(f),
		})
	}
	return ConformanceReport{TargetVersion: target, Findings: findings}
}

// conformanceMigration prefers the finding's own call-site suggestion
// (concrete, derived from the matched code) and falls back to the rule
// catalog's general remediation when no cheap per-call-site fix exists.
func conformanceMigration(f Finding) string {
	if f.Suggestion != "" {
		return f.Suggestion
	}
	if r, ok := rules.Explain(f.RuleID); ok {
		return r.Remediation
	}
	return ""
}
