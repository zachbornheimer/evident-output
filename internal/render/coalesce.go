package render

import (
	"strings"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// ShouldSuppressStandaloneConclusion implements DEC-COAL-* for human projection.
//
// Model and structured JSON always retain independent core.Conclusion + Plan/Changes.
// The trailing human conclusion band is omitted when a planned/changed/ready run
// already has a non-empty Plan or Changes ledger — including Warned runs, whose
// warnings already sit on task "!" rows. Failed/blocked/cancelled/partial and
// Explanation/Actions still keep the band.
//
// See docs/decisions/conclusion-coalescing.md.
func ShouldSuppressStandaloneConclusion(s core.Snapshot) bool {
	if s.Conclusion == nil {
		return false
	}
	c := *s.Conclusion

	if semanticResultCount(s) == 0 {
		return c.Explanation == "" && len(c.Actions) == 0 && !c.Partial && !c.Cancelled && !c.Warned
	}

	// Extra visible conclusion dimensions — never suppress. Warned still
	// keeps the band when there is no Plan/Changes ledger: "· warned" is
	// then the only run-level signal. A non-empty ledger already shows the
	// work, and task "!" rows already carry the warning, so Warned does
	// not keep a content-free title band in that case.
	if c.Explanation != "" || len(c.Actions) > 0 || c.Partial || c.Cancelled {
		return false
	}
	if c.Warned && !hasEffectRecords(s) {
		return false
	}

	// DryRun's own subject header already told the complete story
	// (fixture-repo-retire-dryrun.md: "NO ledger row for tasks/binaries with
	// no effects"): once WriteDryRunMarker rendered s.DryRunSubject and the
	// derived verdict settled on StatePlanned (failed/blocked/partial/
	// cancelled are already excluded above), a trailing "[planned]" band
	// repeats information the header plus the per-section [planned] ledger
	// rows already gave — regardless of how many effect sections exist.
	if s.DryRun && s.DryRunSubject != "" && c.State == core.StatePlanned {
		return true
	}

	if shouldSuppressRepeatedCondition(s, c) {
		return true
	}

	// Severity that is not pure planned/changed success.
	switch c.State {
	case core.StateChanged, core.StatePlanned, core.StateReady:
		// candidates below
	default:
		// failed, blocked, warning, cancelled, partial, …
		return false
	}

	if hasEffectRecords(s) {
		return true
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
		return c.State == core.StateChanged || (c.State == core.StateReady && c.Changed)
	}

	// Exactly one Plan.
	p := s.Plans[0]
	if !sameSemanticSubject(p.Subject, p.ID, c.Subject, s.Subject) {
		return false
	}
	return c.State == core.StatePlanned || c.State == core.StateReady
}

func semanticResultCount(s core.Snapshot) int {
	return len(s.Tasks) + len(s.Collections) + len(s.Changes) + len(s.Plans)
}

func hasEffectRecords(s core.Snapshot) bool {
	for _, ch := range s.Changes {
		if len(ch.Records) > 0 {
			return true
		}
	}
	for _, p := range s.Plans {
		if len(p.Records) > 0 {
			return true
		}
	}
	return false
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
	case core.Blocked:
		return conclusion == core.StateBlocked
	case core.Failed:
		return conclusion == core.StateFailed
	case core.Cancelled:
		return conclusion == core.StateCancelled
	case core.Pending, core.Running, core.Incomplete:
		// Partial is a completeness modifier now, not a headline state
		// (conclusion.go); a lone unresolved entity concludes the same as
		// an empty run — core.StateReady — so that is what "repeats" it.
		return conclusion == core.StateReady
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
		if len(t.Warnings) > 0 {
			return true
		}
		switch t.State {
		case core.Failed, core.Cancelled, core.Incomplete, core.Running, core.Pending:
			return true
		}
	}
	return anyCollectionHasIndependentConditionRows(s.Collections)
}

// anyCollectionHasIndependentConditionRows recurses into every container's
// own children (E2.5 finding 1): a warned grandchild several containers deep
// must count as an independent condition row the same way a root-level
// warned task does, not just the container's own headline State.
func anyCollectionHasIndependentConditionRows(collections []core.TasksSnapshot) bool {
	for _, col := range collections {
		for _, t := range col.Tasks {
			if len(t.Warnings) > 0 {
				return true
			}
			switch t.State {
			case core.Failed, core.Cancelled, core.Incomplete, core.Running, core.Pending:
				return true
			}
		}
		switch col.State {
		case core.Failed, core.Cancelled, core.Incomplete, core.Running, core.Pending:
			return true
		}
		if anyCollectionHasIndependentConditionRows(col.Collections) {
			return true
		}
	}
	return false
}
