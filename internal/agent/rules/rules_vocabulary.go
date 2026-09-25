package rules

// vocabularyRules is the removed-vocabulary family (ZYS-1180 freeze, 1.1):
// each rule flags names the freeze took out of evo and rewrites them to
// the one canonical word for the concept.
func vocabularyRules() []Rule {
	return []Rule{
		{
			ID:              "API-120",
			MinDialect:      "1.1.0",
			Category:        "API",
			Severity:        SeverityError,
			Invariant:       "code uses only exports that serve a concept of the 1.1 vocabulary",
			Why:             "The 1.1 freeze removed exports no caller needed: Reason constraints (ForSkip, OnTask, ReasonOption) that only guarded the removed Kept verb, and a second snapshot JSON encoding beside evo.run and evo.event. They no longer compile.",
			BadCode:         `reason := evo.Reason("dirty", evo.ForSkip())`,
			GoodCode:        `reason := evo.Reason("dirty")`,
			Remediation:     "Delete Reason options (evo.Reason(name) takes only its name); read machine output from evo.WriteJSON (evo.run) or FormatJSONL (evo.event)",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-120"},
			Since:           "1.1.0",
			Certainty:       CertaintyDeterministic,
		},
	}
}

func init() { registerFamily(vocabularyRules()) }
