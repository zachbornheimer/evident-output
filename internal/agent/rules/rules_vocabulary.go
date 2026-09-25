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
			Why:             "ForSkip, OnTask, ReasonOption, ErrReasonSkipOnly, and ErrReasonWrongTask were removed in 1.1: they only guarded the removed Kept verb's Reason constraints and no longer compile.",
			BadCode:         `reason := evo.Reason("dirty", evo.ForSkip())`,
			GoodCode:        `reason := evo.Reason("dirty")`,
			Remediation:     "ForSkip, OnTask, ReasonOption, ErrReasonSkipOnly, and ErrReasonWrongTask were removed in 1.1: delete the option or the errors.Is check, since evo.Reason(name) takes only its name and never returns such an error",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-120"},
			Since:           "1.1.0",
			Certainty:       CertaintyDeterministic,
		},
	}
}

func init() { registerFamily(vocabularyRules()) }
