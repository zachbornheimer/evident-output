package rules

// mcpRules is the MCP server contract (MCP).
func mcpRules() []Rule {
	return []Rule{
		{
			ID:              "MCP-017",
			Category:        "MCP",
			Severity:        SeverityWarning,
			Invariant:       "cross-file package review resolves shared types across files when possible",
			Why:             "Without cross-file resolution, collection-leaf misuse (API-027) and other typed checks can miss real defects hidden behind local type aliases.",
			BadCode:         `// caller submits files independently to GoSource, losing cross-file type info`,
			GoodCode:        `// caller submits the whole package via GoPackage(files) so types resolve across files`,
			Remediation:     "Use GoPackage for multi-file review; treat MCP-017 as a partial-coverage signal, not a defect",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"MCP-017"},
			Since:           "0.4.0",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:              "MCP-021",
			Category:        "MCP",
			Severity:        SeverityError,
			Invariant:       "agents stop only when recheck_required is false",
			Why:             "Stopping while recheck_required is true leaves known defects unfixed.",
			BadCode:         `// agent: one review call then ship`,
			GoodCode:        `// loop: review → repair → review until recheck_required=false`,
			Remediation:     "Loop review until clean; use harness.RunRepairLoop",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"MCP-021", "MCP-022", "MCP-049"},
			Since:           "0.5.0",
			Certainty:       CertaintyDeterministic,
		},
	}
}

func init() { registerFamily(mcpRules()) }
