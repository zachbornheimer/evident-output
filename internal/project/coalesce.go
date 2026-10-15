package project

import (
	"strings"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// ShouldSuppressStandaloneConclusion implements DEC-COAL-* for human projection.
//
// Model and structured JSON always retain independent record.Conclusion + Plan/Changes.
// Only the trailing human conclusion band may be omitted when it repeats a single
// effect section with no extra visible information.
//
// See docs/decisions/conclusion-coalescing.md.
func ShouldSuppressStandaloneConclusion(s record.Snapshot) bool {
	if s.Conclusion == nil {
		return false
	}
	c := *s.Conclusion

	if semanticResultCount(s) == 0 {
		return c.Explanation == "" && len(c.Actions) == 0 && !c.Partial && !c.Cancelled && !c.Warned
	}

	// Extra visible conclusion dimensions — never suppress. Warned joins
	// Partial/Cancelled here (P2): a warned-but-otherwise-suppressible run
	// must not have its "· warned" modifier silently swallowed along with
	// the band it rides on (the same class of gap release-gate round 8
	// finding 3 fixed for the non-suppressed case).
	if c.Explanation != "" || len(c.Actions) > 0 || c.Partial || c.Cancelled || c.Warned {
		return false
	}

	if shouldSuppressRepeatedCondition(s, c) {
		return true
	}

	// Only a Succeeded or Satisfied run can be a pure planned/changed success.
	if c.Outcome != record.OutcomeSucceeded && c.Outcome != record.OutcomeSatisfied {
		return false
	}

	nChanges := len(s.Changes)
	nPlans := len(s.Plans)
	if nChanges+nPlans != 1 {
		return false
	}

	// Any Task/Collection failure or block means conclusion may carry
	// independent severity (or the state switch already failed). Double-check
	// for warning-only tasks that still need a footer.
	if hasIndependentConditionRows(s) {
		return false
	}

	if nChanges == 1 {
		ch := s.Changes[0]
		if !sameSemanticSubject(ch.Subject, ch.ID, c.Subject, s.Subject) {
			return false
		}
		// Compatible: changed (or ready with Changed flag from changes presence).
		return ConclusionHeadline(c) == HeadlineChanged
	}

	// Exactly one Plan.
	p := s.Plans[0]
	if !sameSemanticSubject(p.Subject, p.ID, c.Subject, s.Subject) {
		return false
	}
	return ConclusionHeadline(c) != HeadlineChanged
}

func semanticResultCount(s record.Snapshot) int {
	return len(s.Tasks) + len(s.Collections) + len(s.Changes) + len(s.Plans)
}

func shouldSuppressRepeatedCondition(s record.Snapshot, c record.Conclusion) bool {
	if len(s.Changes)+len(s.Plans) != 0 || len(s.Tasks)+len(s.Collections) != 1 {
		return false
	}

	var name string
	var state record.State
	switch {
	case len(s.Tasks) == 1:
		// I2: a library-synthesized task (Output.Fail/Cancel's "command"
		// fallback for an untracked top-level outcome) is never the caller's
		// own named row — the conclusion band is the ONLY place the run's
		// outcome is stated, so it must never be suppressed as "redundant"
		// with a row the caller never declared.
		if s.Tasks[0].Synthetic() {
			return false
		}
		name, state = s.Tasks[0].Name, s.Tasks[0].State
	default:
		name, state = s.Collections[0].Name, s.Collections[0].State
	}

	subjectRepeatsCondition := c.Subject == "" || normalizeSubject(c.Subject) == normalizeSubject(name)
	return subjectRepeatsCondition && rowStatesHeadline(ConclusionHeadline(c), state)
}

// rowStatesHeadline reports whether a lone row in state already says what a
// band naming headline would. Only a ✓ row does: Succeeded states itself,
// and a Satisfied row says "already satisfied". Every other Outcome is named
// in its own word on the band, so the band stays.
func rowStatesHeadline(headline string, state record.State) bool {
	switch state {
	case record.Succeeded, record.Satisfied:
		return headline == string(state.Outcome())
	}
	return false
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
	// record.Conclusion subject often equals Config.Title while section uses same label.
	return false
}

func normalizeSubject(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}

func hasIndependentConditionRows(s record.Snapshot) bool {
	for _, t := range s.Tasks {
		switch t.State {
		case record.Failed, record.Cancelled, record.Running, record.Pending:
			return true
		}
	}
	return anyCollectionHasIndependentConditionRows(s.Collections)
}

// anyCollectionHasIndependentConditionRows recurses into every container's
// own children (E2.5 finding 1): a failed grandchild several containers deep
// must count as an independent condition row the same way a root-level
// failed task does, not just the container's own headline State.
func anyCollectionHasIndependentConditionRows(collections []record.TasksSnapshot) bool {
	for _, col := range collections {
		for _, t := range col.Tasks {
			switch t.State {
			case record.Failed, record.Cancelled, record.Running, record.Pending:
				return true
			}
		}
		switch col.State {
		case record.Failed, record.Cancelled, record.Running, record.Pending:
			return true
		}
		if anyCollectionHasIndependentConditionRows(col.Collections) {
			return true
		}
	}
	return false
}
