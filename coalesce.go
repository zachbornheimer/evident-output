package evo

import "strings"

// shouldSuppressStandaloneConclusion implements DEC-COAL-* for human projection.
//
// Model and structured JSON always retain independent Conclusion + Plan/Changes.
// Only the trailing human conclusion band may be omitted when it repeats a single
// effect section with no extra visible information.
//
// See docs/decisions/conclusion-coalescing.md.
func shouldSuppressStandaloneConclusion(s Snapshot) bool {
	if s.Conclusion == nil {
		return false
	}
	c := *s.Conclusion

	// Extra visible conclusion dimensions — never suppress.
	if c.Explanation != "" || len(c.Actions) > 0 || c.Partial || c.Cancelled {
		return false
	}
	// Severity that is not pure planned/changed success.
	switch c.State {
	case StateChanged, StatePlanned, StateReady, StateUnchanged:
		// candidates below
	default:
		// failed, blocked, warning, cancelled, partial, …
		return false
	}

	nChanges := len(s.Changes)
	nPlans := len(s.Plans)
	if nChanges+nPlans != 1 {
		return false
	}

	// Any Item/Task/Collection failure or block means conclusion may carry
	// independent severity (or the state switch already failed). Double-check
	// for warning-only items that still need a footer.
	if hasIndependentConditionRows(s) {
		return false
	}

	if nChanges == 1 {
		ch := s.Changes[0]
		if !sameSemanticSubject(ch.Subject, ch.ID, c.Subject, s.Subject) {
			return false
		}
		// Compatible: changed (or ready with Changed flag from changes presence).
		return c.State == StateChanged || (c.State == StateReady && c.Changed) || c.State == StateUnchanged
	}

	// Exactly one Plan.
	p := s.Plans[0]
	if !sameSemanticSubject(p.Subject, p.ID, c.Subject, s.Subject) {
		return false
	}
	return c.State == StatePlanned || c.State == StateReady || c.State == StateUnchanged
}

// sameSemanticSubject compares effect-section identity to conclusion/output subject.
// Prefer non-empty IDs when both sides have them; otherwise normalized display subjects.
func sameSemanticSubject(sectionSubject, sectionID, conclusionSubject, outputSubject string) bool {
	// Prefer explicit section ID matching output subject key when both look like keys.
	if sectionID != "" && (sectionID == conclusionSubject || sectionID == outputSubject) {
		return true
	}
	sec := normalizeSubject(sectionSubject)
	con := normalizeSubject(conclusionSubject)
	out := normalizeSubject(outputSubject)
	if sec == "" {
		return false
	}
	if con != "" && sec == con {
		return true
	}
	if out != "" && sec == out {
		return true
	}
	// Conclusion subject often equals Config.Title while section uses same label.
	return false
}

func normalizeSubject(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}

func hasIndependentConditionRows(s Snapshot) bool {
	for _, it := range s.Items {
		switch it.State {
		case Failed, Blocked, Warning, Incomplete, Running, Pending:
			return true
		}
	}
	for _, t := range s.Tasks {
		switch t.State {
		case Failed, Warning, Cancelled, Incomplete, Running, Pending:
			return true
		}
	}
	for _, col := range s.Collections {
		switch col.State {
		case Failed, Warning, Cancelled, Incomplete, Running, Pending:
			return true
		}
	}
	return false
}
