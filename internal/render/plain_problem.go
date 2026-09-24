package render

import (
	"fmt"
	"strings"

	txt "github.com/zachbornheimer/evident-output/internal/text"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// writeProblem renders one problem row. emphasize is true for a core.Failed/core.Blocked
// task's evidence (release-gate round 6 finding 5): the ├─/│/evidence-glyph
// connectors stay txt.Dim either way — they are decoration — but the evidence
// text itself renders at full intensity so a failure's proof is never the
// lowest-contrast text on screen.
func writeProblem(b *strings.Builder, p core.Problem, color, emphasize bool, profile txt.GlyphProfile) {
	detail, tail := effectiveDetailAndTail(p)
	if p.Subject != "" {
		extra := p.Summary
		if p.Count != 0 {
			extra = fmt.Sprintf("%s (%d)", p.Summary, p.Count)
		}
		fmt.Fprintf(b, "%s%s %s  %s\n", problemTreeIndent, txt.Dim("├─", color), p.Subject, extra)
		if detail != "" {
			writeProblemDetailLines(b, detail, txt.Dim("│", color), color, emphasize)
		}
		if tail != "" {
			writeProblemDetailLines(b, tail, txt.Dim("│", color), color, emphasize)
		}
		return
	}
	// Detail present: preserve multi-line body (P3). Summary heads the block only
	// when the caller left it set — writeTask clears Summary when it already
	// appears on the ✗ row so Detail alone is the evidence body (P4).
	if detail != "" {
		writeProblemDetailBlock(b, p.Summary, detail, color, emphasize, profile)
		if tail != "" {
			writeAdditionalEvidenceLines(b, tail, color, emphasize)
		}
		return
	}
	fmt.Fprintf(b, "%s%s %s\n", problemTreeIndent, txt.Dim(txt.GlyphEvidence.Render(profile), color), emphasizedText(p.Summary, color, emphasize))
}

// dedupeEvidenceTailAgainstRow is P7's addition (user-13-problems.md
// Problem 7: "deduplicate it against the failure message"). The exact
// anti-pattern the doc names — task.Failf("install failed: %s",
// capture.Text()) — folds the retained output straight into the row's own
// summary; auto-attach (task.go's finish) still sets EvidenceTail from the
// same capture ring, which would otherwise render that text a second time
// underneath the row. Clearing it here (a rendering decision, made once
// output already has both values in hand) rather than at attach time avoids
// a real hazard: finish() holds Output's lock while auto-attaching, and
// re-entering the capture ring's redactor lock from a second, later attempt
// deadlocks (see task.go's finish doc comment) — so the Problem's stored
// EvidenceTail must stay exactly as captured, and only the row rendering it
// decides not to repeat what the row headline already said.
func dedupeEvidenceTailAgainstRow(p core.Problem, rowSummary string) core.Problem {
	if p.EvidenceTail == "" || rowSummary == "" {
		return p
	}
	if strings.Contains(rowSummary, p.EvidenceTail) {
		p.EvidenceTail = ""
	}
	return p
}

// emphasizedText applies txt.Dim unless emphasize opts the text out of demotion —
// the single place that decides "is this text decoration or evidence" for
// the problem-rendering chain.
func emphasizedText(s string, color, emphasize bool) string {
	if emphasize {
		return s
	}
	return txt.Dim(s, color)
}

// effectiveDetailAndTail resolves a core.Problem's Detail and EvidenceTail into
// the pair actually rendered: an explicit Detail always renders (never
// silently discarded by an auto-attached or explicitly requested evidence
// tail), and a distinct EvidenceTail renders as an additional evidence line
// underneath it. When Detail is empty, EvidenceTail alone renders as the
// detail body — DetailTail's original, still-supported shape. An identical
// EvidenceTail (auto-attach filled Detail with the same capture tail a
// caller also passed explicitly via DetailTail) collapses to one line, not a
// duplicate.
func effectiveDetailAndTail(p core.Problem) (detail, tail string) {
	switch {
	case p.Detail == "":
		return p.EvidenceTail, ""
	case p.EvidenceTail == "" || p.EvidenceTail == p.Detail:
		return p.Detail, ""
	default:
		return p.Detail, p.EvidenceTail
	}
}

// writeAdditionalEvidenceLines renders tail's lines as continuation rows
// under a just-written Detail block, matching writeProblemDetailBlock's own
// continuation indent so the tail reads as more evidence for the same
// problem rather than a new one.
func writeAdditionalEvidenceLines(b *strings.Builder, tail string, color, emphasize bool) {
	for _, line := range splitPresentationLines(tail) {
		fmt.Fprintf(b, "%s%s\n", problemDetailIndent, emphasizedText(line, color, emphasize))
	}
}

// writeProblemDetailBlock renders Detail as an indented multi-line block under
// the evidence connector. When summary is non-empty it opens the block;
// continuations (and all detail lines when summary is empty) are indented
// under it.
func writeProblemDetailBlock(b *strings.Builder, summary, detail string, color, emphasize bool, profile txt.GlyphProfile) {
	lines := splitPresentationLines(detail)
	evidence := txt.Dim(txt.GlyphEvidence.Render(profile), color)
	if summary != "" {
		fmt.Fprintf(b, "%s%s %s\n", problemTreeIndent, evidence, emphasizedText(summary, color, emphasize))
		for _, line := range lines {
			fmt.Fprintf(b, "%s%s\n", problemDetailIndent, emphasizedText(line, color, emphasize))
		}
		return
	}
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(b, "%s%s %s\n", problemTreeIndent, evidence, emphasizedText(lines[0], color, emphasize))
	for _, line := range lines[1:] {
		fmt.Fprintf(b, "%s%s\n", problemDetailIndent, emphasizedText(line, color, emphasize))
	}
}

// writeProblemDetailLines continues Detail under a subject (├─) row with │ prefixes.
func writeProblemDetailLines(b *strings.Builder, detail, pipe string, color, emphasize bool) {
	for _, line := range splitPresentationLines(detail) {
		fmt.Fprintf(b, "%s%s %s\n", problemTreeIndent, pipe, emphasizedText(line, color, emphasize))
	}
}

// splitPresentationLines splits on \n and drops a single trailing empty segment
// so a trailing newline does not produce a blank residual row.
func splitPresentationLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

// writeAction renders one next-action row prefixed by the profile-aware next-
// action glyph (→ / >). evo-rec.md's tightened vocabulary gives "next action"
// its own row so the meaning does not rest on cyan color alone.
func writeAction(b *strings.Builder, a core.Action, color bool, profile txt.GlyphProfile) {
	glyph := txt.StyleGlyph(txt.GlyphNextAction.Render(profile), txt.SGRCyan, color)
	if a.Command != nil {
		cmd := a.Command.Executable + " " + strings.Join(a.Command.Args, " ")
		fmt.Fprintf(b, "%s  %s\n", glyph, txt.Style(cmd, txt.SGRCyan, color))
		return
	}
	if a.Label != "" {
		fmt.Fprintf(b, "%s  %s\n", glyph, a.Label)
	}
}
