package render

import (
	"strings"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// ShouldSuppressStandaloneConclusion implements DEC-COAL-* for human projection.
//
// Model and structured JSON always retain independent core.Conclusion + Plan/Changes.
// Only the trailing human conclusion band may be omitted when it repeats a single
// effect section with no extra visible information.
//
// See docs/decisions/conclusion-coalescing.md.
func ShouldSuppressStandaloneConclusion(s core.Snapshot) bool {
	if s.Conclusion == nil {
		return false
	}
	c := *s.Conclusion

	if semanticResultCount(s) == 0 {
		return c.Explanation == "" && len(c.Actions) == 0 && !c.Partial && !c.Cancelled
	}

	// Extra visible conclusion dimensions — never suppress.
	if c.Explanation != "" || len(c.Actions) > 0 || c.Partial || c.Cancelled {
		return false
	}
	if shouldSuppressRepeatedCondition(s, c) {
		return true
	}

	// Severity that is not pure planned/changed success.
	switch c.State {
	case core.StateChanged, core.StatePlanned, core.StateReady, core.StateUnchanged:
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
		return c.State == core.StateChanged || (c.State == core.StateReady && c.Changed) || c.State == core.StateUnchanged
	}

	// Exactly one Plan.
	p := s.Plans[0]
	if !sameSemanticSubject(p.Subject, p.ID, c.Subject, s.Subject) {
		return false
	}
	return c.State == core.StatePlanned || c.State == core.StateReady || c.State == core.StateUnchanged
}

func semanticResultCount(s core.Snapshot) int {
	return len(s.Tasks) + len(s.Collections) + len(s.Changes) + len(s.Plans)
}

func shouldSuppressRepeatedCondition(s core.Snapshot, c core.Conclusion) bool {
	if len(s.Changes)+len(s.Plans) != 0 || len(s.Tasks)+len(s.Collections) != 1 {
		return false
	}

	var name string
	var state core.EntityState
	switch {
	case len(s.Tasks) == 1:
		// I2: a library-synthesized task (Output.Failf/Cancel's "command"
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
	return subjectRepeatsCondition && conclusionRepeatsEntityState(c.State, state)
}

func conclusionRepeatsEntityState(conclusion core.ConclusionState, entity core.EntityState) bool {
	switch entity {
	case core.Done, core.Skipped, core.Empty:
		return conclusion == core.StateReady
	case core.Warning:
		return conclusion == core.StateWarning
	case core.Blocked:
		return conclusion == core.StateBlocked
	case core.Failed:
		return conclusion == core.StateFailed
	case core.Cancelled:
		return conclusion == core.StateCancelled
	case core.Pending, core.Running, core.Incomplete:
		// Partial is a completeness modifier now, not a headline state
		// (conclusion.go); a lone unresolved entity concludes the same as
		// an empty run — core.StateUnchanged — so that is what "repeats" it.
		return conclusion == core.StateUnchanged
	default:
		return false
	}
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
	// core.Conclusion subject often equals Config.Title while section uses same label.
	return false
}

func normalizeSubject(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}

func hasIndependentConditionRows(s core.Snapshot) bool {
	for _, t := range s.Tasks {
		switch t.State {
		case core.Failed, core.Warning, core.Cancelled, core.Incomplete, core.Running, core.Pending:
			return true
		}
	}
	for _, col := range s.Collections {
		switch col.State {
		case core.Failed, core.Warning, core.Cancelled, core.Incomplete, core.Running, core.Pending:
			return true
		}
	}
	return false
}
