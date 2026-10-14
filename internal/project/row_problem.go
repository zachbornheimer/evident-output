package project

import (
	"fmt"
	"slices"
	"strings"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// writeProblem renders one problem row, indent being its task row's own
// prefix, so a child's Problems sit under the child glyph as a root row's
// sit under its own. emphasize is true for a record.Failed/record.Refused
// task's evidence (release-gate round 6 finding 5): the ├─/│/evidence-glyph
// connectors stay Dim either way — they are decoration — but the evidence
// text itself renders at full intensity so a failure's proof is never the
// lowest-contrast text on screen.
//
// closes is true only for the branch that ends the row's evidence tree: it
// takes the evidence glyph rather than leaving a branch dangling, and every
// other branch keeps the tree open with ├─ and │.
func writeProblem(b *strings.Builder, p record.Problem, indent string, emphasize, closes bool, s Style) {
	detail, tail := effectiveDetailAndTail(p)
	branch := newEvidenceBranch(closes, s)
	if p.Subject != "" {
		extra := p.Summary
		if p.Count != 0 {
			extra = strings.TrimSpace(fmt.Sprintf("%s (%d)", p.Summary, p.Count))
		}
		subject := p.Subject
		if extra != "" {
			subject += "  " + extra
		}
		fmt.Fprintf(b, "%s%s%s %s\n", indent, ProblemTreeIndent, branch.glyph, subject)
		for _, text := range []string{detail, tail} {
			writeProblemDetailLines(b, text, branch.continuation, indent, emphasize, s)
		}
		return
	}
	// Detail present: preserve multi-line body (P3). Summary heads the block only
	// when the caller left it set — writeTask clears Summary when it already
	// appears on the ✗ row so Detail alone is the evidence body (P4).
	if detail != "" {
		writeProblemDetailBlock(b, p.Summary, detail, indent, branch, emphasize, s)
		if tail != "" {
			writeAdditionalEvidenceLines(b, tail, indent, branch, emphasize, s)
		}
		return
	}
	fmt.Fprintf(b, "%s%s%s %s\n", indent, ProblemTreeIndent, branch.glyph, s.Emphasized(p.Summary, emphasize))
}

// evidenceBranch is the connector pair of one branch of a row's evidence
// tree: the glyph that opens the branch and the continuation column that
// carries its further lines. A branch that closes the tree opens with the
// evidence glyph and continues in blank space; any other opens with ├─ and
// continues under │.
type evidenceBranch struct {
	glyph        string
	continuation string
}

// newEvidenceBranch is the connector pair for a branch that closes the tree
// when closes is true and keeps it open otherwise.
func newEvidenceBranch(closes bool, s Style) evidenceBranch {
	if closes {
		return evidenceBranch{glyph: s.EvidenceGlyph(), continuation: " "}
	}
	return evidenceBranch{glyph: s.BranchGlyph(), continuation: s.Dim("│")}
}

// detailIndent is the column a branch's continuation lines start at, beneath
// the branch's own text.
func (e evidenceBranch) detailIndent(indent string) string {
	return indent + ProblemTreeIndent + e.continuation + problemDetailGap
}

// dedupeEvidenceTailAgainstRow is P7's addition (user-13-problems.md
// Problem 7: "deduplicate it against the failure message"). The exact
// anti-pattern the doc names — task.Fail("install failed", evo.Detail(
// capture.Text())) — folds the retained output straight into the row's own
// summary; auto-attach (task.go's finish) still sets EvidenceTail from the
// same capture ring, which would otherwise render that text a second time
// underneath the row. Clearing it here (a rendering decision, made once
// output already has both values in hand) rather than at attach time avoids
// a real hazard: finish() holds Output's lock while auto-attaching, and
// re-entering the capture ring's redactor lock from a second, later attempt
// deadlocks (see task.go's finish doc comment) — so the Problem's stored
// EvidenceTail must stay exactly as captured, and only the row rendering it
// decides not to repeat what the row headline already said.
func dedupeEvidenceTailAgainstRow(p record.Problem, rowSummary string) record.Problem {
	if p.EvidenceTail == "" || rowSummary == "" {
		return p
	}
	if strings.Contains(rowSummary, p.EvidenceTail) {
		p.EvidenceTail = ""
	}
	return p
}

// withoutRestatedAttributes is p without the Detail and EvidenceTail lines
// that say a failed attribute in details failed: its own branch of the row's
// evidence tree already does, so the tree states each failure once.
func withoutRestatedAttributes(p record.Problem, details []record.VerificationDetail) record.Problem {
	var restated []string
	for _, d := range details {
		if d.Status != record.VerificationSatisfied {
			restated = append(restated, record.FailedAttributeText(d.Name))
		}
	}
	if len(restated) == 0 {
		return p
	}
	p.Detail = withoutLines(p.Detail, restated)
	p.EvidenceTail = withoutLines(p.EvidenceTail, restated)
	return p
}

// withoutLines is text without the lines equal to any of drop.
func withoutLines(text string, drop []string) string {
	lines := splitPresentationLines(text)
	kept := slices.DeleteFunc(slices.Clone(lines), func(line string) bool { return slices.Contains(drop, line) })
	if len(kept) == len(lines) {
		return text
	}
	return strings.Join(kept, "\n")
}

// effectiveDetailAndTail resolves a record.Problem's Detail and EvidenceTail into
// the pair actually rendered: an explicit Detail always renders (never
// silently discarded by an auto-attached or explicitly requested evidence
// tail), and a distinct EvidenceTail renders as an additional evidence line
// underneath it. When Detail is empty, EvidenceTail alone renders as the
// detail body — DetailTail's original, still-supported shape. An identical
// EvidenceTail (auto-attach filled Detail with the same capture tail a
// caller also passed explicitly via DetailTail) collapses to one line, not a
// duplicate.
func effectiveDetailAndTail(p record.Problem) (detail, tail string) {
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
func writeAdditionalEvidenceLines(b *strings.Builder, tail, indent string, branch evidenceBranch, emphasize bool, s Style) {
	for _, line := range splitPresentationLines(tail) {
		fmt.Fprintf(b, "%s%s\n", branch.detailIndent(indent), s.Emphasized(line, emphasize))
	}
}

// writeProblemDetailBlock renders Detail as an indented multi-line block under
// the evidence connector. When summary is non-empty it opens the block;
// continuations (and all detail lines when summary is empty) are indented
// under it.
func writeProblemDetailBlock(b *strings.Builder, summary, detail, indent string, branch evidenceBranch, emphasize bool, s Style) {
	lines := splitPresentationLines(detail)
	if summary == "" {
		if len(lines) == 0 {
			return
		}
		summary, lines = lines[0], lines[1:]
	}
	fmt.Fprintf(b, "%s%s%s %s\n", indent, ProblemTreeIndent, branch.glyph, s.Emphasized(summary, emphasize))
	for _, line := range lines {
		fmt.Fprintf(b, "%s%s\n", branch.detailIndent(indent), s.Emphasized(line, emphasize))
	}
}

// writeProblemDetailLines continues Detail under a subject row, each line
// led by the continuation connector (│ under ├─, blank under the closing └─).
func writeProblemDetailLines(b *strings.Builder, detail, continuation, indent string, emphasize bool, s Style) {
	for _, line := range splitPresentationLines(detail) {
		fmt.Fprintf(b, "%s%s%s %s\n", indent, ProblemTreeIndent, continuation, s.Emphasized(line, emphasize))
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

// WriteAction renders one next-action row prefixed by the profile-aware next-
// action glyph (→ / >). evo-rec.md's tightened vocabulary gives "next action"
// its own row so the meaning does not rest on cyan color alone.
func WriteAction(b *strings.Builder, a record.Action, s Style) {
	glyph := StyleGlyph(GlyphNextAction.Render(s.Profile), SGRCyan, s.Color)
	if a.Command != nil {
		cmd := a.Command.Executable + " " + strings.Join(a.Command.Args, " ")
		fmt.Fprintf(b, "%s  %s\n", glyph, s.Paint(cmd, SGRCyan))
		return
	}
	if a.Label != "" {
		fmt.Fprintf(b, "%s  %s\n", glyph, a.Label)
	}
}
