package render

import (
	"fmt"
	"strings"

	txt "github.com/zachbornheimer/evident-output/internal/text"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// WritePlannedHeader emits a planned run's unmissable opening line. It
// renders once, first — a caller cannot opt out or bury it, because every
// planned projection (RenderPlain and Finish's residual) calls this before
// any other row.
//
// subject is Config.Subject's text (fixture-repo-retire-dryrun.md's "repo
// <path>"), merged onto this one line instead of streaming as a second
// durable line — a blank line separates the header from whatever follows,
// matching the fixture's "ONE header line ... blank line after". Empty
// subject falls back to the plain announcement text, unchanged from before
// Config.Subject existed.
//
// preview drops the tag entirely: a preview before a confirm gate announces
// the subject it is about to ask about ("repo <path>"), because telling the
// user nothing will happen and then asking them to authorize it is the
// contradiction the dialect's screenshot regression names. A preview with no
// subject has nothing to announce and writes nothing.
func WritePlannedHeader(b *strings.Builder, color, preview bool, subject string) {
	if preview {
		if subject != "" {
			fmt.Fprintf(b, "%s\n\n", subject)
		}
		return
	}
	tag := txt.Style("[dry-run]", effectColor("planned"), color)
	body := dryRunMarkerText
	if subject != "" {
		body = subject
	}
	fmt.Fprintf(b, "%s %s\n\n", tag, body)
}

// conclusionBandTag renders the trailing outcome band's bracketed tag,
// appending conclusionPartialModifier and/or conclusionWarnedModifier when
// the conclusion carries either — evo-rec.md's spec does not pin a literal
// spelling for these modifier bands (only the two/three-axis rule), so the
// modifier form here is the documented implementation choice (see work
// order for the corresponding spec-edit note).
func conclusionBandTag(c core.Conclusion) string {
	tag := string(c.State)
	if c.Partial {
		tag += conclusionPartialModifier
	}
	if c.Warned {
		tag += conclusionWarnedModifier
	}
	return fmt.Sprintf("[%s]", tag)
}

func WriteConclusion(b *strings.Builder, c core.Conclusion, s Style) {
	if c.State == core.StateCancelled {
		writeCancellationBand(b, c, s)
		return
	}
	tag := s.Paint(conclusionBandTag(c), conclusionColor(c.State))
	// A bare Subject that equals the Headline state word itself ("changed",
	// "failed", ...) says nothing the bracketed tag hasn't already said — it
	// is what an unconfigured Config.Title falls back to, not a caller's
	// chosen subject, so printing it stutters the band ("[changed]  changed",
	// release-gate round 10 finding 1). Suppress it instead of repeating it.
	if c.Subject != "" && c.Subject != string(c.State) {
		fmt.Fprintf(b, "\n%s  %s\n", tag, s.Paint(c.Subject, txt.SGRBold))
	} else {
		fmt.Fprintf(b, "\n%s\n", tag)
	}
	if c.Explanation != "" {
		fmt.Fprintf(b, "  %s\n", c.Explanation)
	}
	if c.State == core.StateFailed {
		writeAlreadyMutated(b, c.Changes, s)
	}
	for _, a := range c.Actions {
		writeAction(b, a, s)
	}
}

// writeCancellationBand renders the cancelled outcome as contract §15 shows
// it: "[cancelled] <subject>  <cause>", then the partial-changes note only
// when some Effect committed. The cause is the Conclusion's Explanation (for
// example "by user"), carried on the band line itself instead of a second
// sentence beneath it.
func writeCancellationBand(b *strings.Builder, c core.Conclusion, s Style) {
	// "cancelled" already says the run stopped short; a "· partial" modifier
	// beside it would only repeat that (the not-started rows say which part).
	tagged := c
	tagged.Partial = false
	line := s.Paint(conclusionBandTag(tagged), conclusionColor(c.State))
	if c.Subject != "" && c.Subject != string(c.State) {
		line += " " + s.Paint(c.Subject, txt.SGRBold)
	}
	if c.Explanation != "" {
		line += "  " + c.Explanation
	}
	fmt.Fprintf(b, "\n%s\n", line)
	if _, committed := summarizeAlreadyMutated(c.Changes); committed {
		fmt.Fprintf(b, "  %s %s\n", s.warningGlyph(), cancellationPartialChangesNote)
	}
	for _, a := range c.Actions {
		writeAction(b, a, s)
	}
}

func conclusionColor(s core.ConclusionState) string {
	switch s {
	case core.StateReady, core.StateChanged:
		return txt.SGRGreen
	case core.StatePlanned:
		return txt.SGRBlue
	case core.StateFailed:
		return txt.SGRRed
	case core.StateBlocked:
		return txt.SGRRed
	case core.StateCancelled:
		return txt.SGRDim
	default:
		return txt.SGRCyan
	}
}

func effectColor(kind string) string {
	switch kind {
	case "changed":
		return txt.SGRGreen
	case "planned":
		return txt.SGRBlue
	default:
		return txt.SGRCyan
	}
}
