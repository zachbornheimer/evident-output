package rules

// progressRules is the live-count family (1.1): Progress is the count and
// Doing names the current item (API-090).
func progressRules() []Rule {
	return []Rule{
		{
			ID:              "API-090",
			MinDialect:      "1.1.0",
			Category:        "API",
			Severity:        SeverityWarning,
			Invariant:       "a call site never uses TaskHandle.Step (removed in 1.1); a count's current item is Progress(completed, total).Doing(item)",
			Why:             "Progress is the one count verb and Doing the one current-activity verb (contract Vocabulary: \"Progress wins over Step\"). Step fused the two into a third spelling that taught nothing either verb did not already say. After Progress or Bytes, a plain transcript shows Doing's item only on a thinned progress milestone's line, never a line per item, the property Step used to own.",
			BadCode:         `task.Step(i, len(paths), path)`,
			GoodCode:        `task.Progress(i, len(paths)).Doing(path)`,
			Remediation:     "Replace task.Step(completed, total, item) with task.Progress(completed, total).Doing(item)",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-090"},
			Since:           "1.1.0",
			Certainty:       CertaintyDeterministic,
		},
	}
}

func init() { registerFamily(progressRules()) }
