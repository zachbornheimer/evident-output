package plain

import (
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// WritePlannedHeader emits a planned run's unmissable opening line. It
// renders once, first — a caller cannot opt out or bury it, because every
// planned projection (RenderPlain and Finish's residual) calls this before
// any other row. A blank line separates it from whatever follows.
func WritePlannedHeader(b *strings.Builder, color bool) {
	tag := project.ApplySGR("[dry-run]", project.EffectColor("planned"), color)
	fmt.Fprintf(b, "%s %s\n\n", tag, dryRunMarkerText)
}

// conclusionBandTag renders the trailing outcome band's bracketed tag,
// appending conclusionPartialModifier and/or conclusionWarnedModifier when
// the conclusion carries either — evo-rec.md's spec does not pin a literal
// spelling for these modifier bands (only the two/three-axis rule), so the
// modifier form here is the documented implementation choice (see work
// order for the corresponding spec-edit note).
func conclusionBandTag(c record.Conclusion) string {
	tag := project.ConclusionHeadline(c)
	if c.Partial {
		tag += conclusionPartialModifier
	}
	if c.Warned {
		tag += conclusionWarnedModifier
	}
	return fmt.Sprintf("[%s]", tag)
}

func WriteConclusion(b *strings.Builder, c record.Conclusion, s project.Style) {
	if c.Outcome == record.OutcomeCancelled {
		writeCancellationBand(b, c, s)
		return
	}
	tag := s.Paint(conclusionBandTag(c), project.HeadlineColor(project.ConclusionHeadline(c)))
	// A bare Subject that equals the headline state word itself ("changed",
	// "failed", ...) says nothing the bracketed tag hasn't already said — it
	// is what an unconfigured Config.Title falls back to, not a caller's
	// chosen subject, so printing it stutters the band ("[changed]  changed",
	// release-gate round 10 finding 1). Suppress it instead of repeating it.
	if c.Subject != "" && c.Subject != project.ConclusionHeadline(c) {
		fmt.Fprintf(b, "\n%s  %s\n", tag, s.Paint(c.Subject, project.SGRBold))
	} else {
		fmt.Fprintf(b, "\n%s\n", tag)
	}
	// The Explanation is not repeated here: it is the deciding Task's ending,
	// which that Task's own row already states.
	if c.Outcome == record.OutcomeFailed {
		project.WriteAlreadyMutated(b, c.Changes, s)
	}
	for _, a := range c.Actions {
		project.WriteAction(b, a, s)
	}
}

// writeCancellationBand renders the cancelled outcome as CONTRACT §4
// shows it: "[cancelled]  <subject>  <cause>", then the partial-changes note only
// when some Effect committed. The cause is the Conclusion's Explanation (for
// example "by user"), carried on the band line itself instead of a second
// sentence beneath it.
func writeCancellationBand(b *strings.Builder, c record.Conclusion, s project.Style) {
	// "cancelled" already says the run stopped short; a "· partial" modifier
	// beside it would only repeat that (the not-started rows say which part).
	tagged := c
	tagged.Partial = false
	line := s.Paint(conclusionBandTag(tagged), project.HeadlineColor(project.ConclusionHeadline(c)))
	if c.Subject != "" && c.Subject != project.ConclusionHeadline(c) {
		line += "  " + s.Paint(c.Subject, project.SGRBold)
	}
	if c.Explanation != "" {
		line += "  " + c.Explanation
	}
	fmt.Fprintf(b, "\n%s\n", line)
	if _, committed := project.SummarizeAlreadyMutated(c.Changes); committed {
		fmt.Fprintf(b, "  %s %s\n", s.WarningGlyph(), project.CancellationPartialChangesNote)
	}
	for _, a := range c.Actions {
		project.WriteAction(b, a, s)
	}
}

// dryRunMarkerText is the fixed announcement body for a dry-run run's
// opening line (evo-rec.md record.Problem 1: "a dry run must announce itself").
const dryRunMarkerText = "no changes will be made"

// conclusionPartialModifier is the literal suffix that marks the printed
// band as evo-rec.md's completeness axis rather than a new headline: a run
// that never invented a State of its own (StatePartial is dead precisely
// because Partial is a modifier, not a root verdict) still needs an honest
// band when record.Conclusion.Partial is true (release-gate round 4 finding 1) — an
// abandoned per-item loop or a forgotten terminal verb on an otherwise clean
// finish must not read as silently complete.
const conclusionPartialModifier = " · partial"

// conclusionWarnedModifier marks the printed band with the same "modifier,
// not a new headline" treatment as conclusionPartialModifier (release-gate
// round 8 finding 3): a run that carries at least one warning-severity
// Problem (P2: a warning never resolves its own lifecycle state; Warn was
// removed in 1.1) while its
// headline settled on an OK-family state (e.g. [ready]) must not read as
// silently clean — the exit code is unchanged, only the band gains this
// suffix. record.Conclusion.Warned is already false when State is itself
// record.StateWarning, so the two never double up.
const conclusionWarnedModifier = " · warned"
