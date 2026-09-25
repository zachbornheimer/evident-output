// Package review — API-120: exports the 1.1 freeze removed because they
// served no concept a caller needs (E-122). Entries come from
// retired.ReasonVocabularyRemovals, the one table shared with the MCP
// migration rule in internal/agent/rules/rules_vocabulary.go, so the
// names, rule ID, and rewrite cannot drift.
package review

import "github.com/zachbornheimer/evident-output/internal/retired"

func init() {
	for _, r := range retired.ReasonVocabularyRemovals {
		registerRemoved(removedName{
			rule:       r.RuleID,
			name:       r.Name,
			message:    "evo." + r.Name + " was removed in 1.1: " + r.Note,
			suggestion: "delete it: evo.Reason(name) takes only its name",
		})
	}
}
