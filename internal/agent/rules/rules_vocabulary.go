package rules

import (
	"strings"

	"github.com/zachbornheimer/evident-output/internal/retired"
)

// vocabularyRules is the removed-vocabulary family (ZYS-1180 freeze, 1.1):
// one rule (API-120) flags every name the freeze took out of evo.Reason's
// usage-constraint surface and rewrites it to the bare evo.Reason(name)
// form. Names come from retired.ReasonVocabularyRemovals, the one table
// shared with the structural detector in
// internal/agent/review/review_removed_exports.go, so the two cannot
// drift.
func vocabularyRules() []Rule {
	names := make([]string, len(retired.ReasonVocabularyRemovals))
	for i, r := range retired.ReasonVocabularyRemovals {
		names[i] = r.Name
	}
	list := strings.Join(names, ", ")
	return []Rule{
		{
			ID:              "API-120",
			MinDialect:      "1.1.0",
			Category:        "API",
			Severity:        SeverityError,
			Invariant:       "code uses only exports that serve a concept of the 1.1 vocabulary",
			Why:             list + " were removed in 1.1: they only guarded Reason usage constraints and no longer compile.",
			BadCode:         `reason := evo.Reason("dirty", evo.ForSkip())`,
			GoodCode:        `reason := evo.Reason("dirty")`,
			Remediation:     list + " were removed in 1.1: delete the option or the errors.Is check, since evo.Reason(name) takes only its name and never returns such an error",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-120"},
			Since:           "1.1.0",
			Certainty:       CertaintyDeterministic,
		},
	}
}

func init() { registerFamily(vocabularyRules()) }
