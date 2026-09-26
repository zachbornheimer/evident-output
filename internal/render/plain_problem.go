package render

import (
	"fmt"
	"strings"

	txt "github.com/zachbornheimer/evident-output/internal/text"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// writeProblem renders one problem row, indent being its task row's own
// prefix, so a child's Problems sit under the child glyph as a root row's
// sit under its own. emphasize is true for a core.Failed/core.Blocked
// task's evidence (release-gate round 6 finding 5): the ├─/│/evidence-glyph
// connectors stay txt.Dim either way — they are decoration — but the evidence
// text itself renders at full intensity so a failure's proof is never the
// lowest-contrast text on screen.
func writeProblem(b *strings.Builder, p core.Problem, indent string, emphasize bool, s Style) {
	detail, tail := effectiveDetailAndTail(p)
	if p.Subject != "" {
		extra := p.Summary
		if p.Count != 0 {
			extra = fmt.Sprintf("%s (%d)", p.Summary, p.Count)
		}
		fmt.Fprintf(b, "%s%s%s %s  %s\n", indent, problemTreeIndent, s.dim("├─"), p.Subject, extra)
		if detail != "" {
			writeProblemDetailLines(b, detail, indent, emphasize, s)
		}
		if tail != "" {
			writeProblemDetailLines(b, tail, indent, emphasize, s)
		}
		return
	}
	// Detail present: preserve multi-line body (P3). Summary heads the block only
	// when the caller left it set — writeTask clears Summary when it already
	// appears on the ✗ row so Detail alone is the evidence body (P4).
	if detail != "" {
		writeProblemDetailBlock(b, p.Summary, detail, indent, emphasize, s)
		if tail != "" {
			writeCaptureTailLines(b, tail, indent, emphasize, s)
		}
		return
	}
	fmt.Fprintf(b, "%s%s%s %s\n", indent, problemTreeIndent, s.evidenceGlyph(), s.emphasized(p.Summary, emphasize))
}

// dedupeCaptureTailAgainstRow is P7's addition (user-13-problems.md
// Problem 7: "deduplicate it against the failure message"). The exact
// anti-pattern the doc names — task.Fail(fmt.Sprintf("install failed: %s",
// capture.Text())) — folds the retained output straight into the row's own
// summary; auto-attach (task.go's finish) still sets CaptureTail from the
// same capture ring, which would otherwise render that text a second time
// underneath the row. Clearing it here (a rendering decision, made once
// output already has both values in hand) rather than at attach time avoids
// a real hazard: finish() holds Output's lock while auto-attaching, and
// re-entering the capture ring's redactor lock from a second, later attempt
// deadlocks (see task.go's finish doc comment) — so the Problem's stored
// CaptureTail must stay exactly as captured, and only the row rendering it
// decides not to repeat what the row headline already said.
func dedupeCaptureTailAgainstRow(p core.Problem, rowSummary string) core.Problem {
	if p.CaptureTail == "" || rowSummary == "" {
		return p
	}
	if strings.Contains(rowSummary, p.CaptureTail) {
		p.CaptureTail = ""
	}
	return p
}

// effectiveDetailAndTail resolves a core.Problem's Detail and CaptureTail into
// the pair actually rendered: an explicit Detail always renders (never
// silently discarded by an auto-attached or explicitly requested capture
// tail), and a distinct CaptureTail renders as an additional capture line
// underneath it. When Detail is empty, CaptureTail alone renders as the
// detail body — DetailTail's original, still-supported shape. attachCaptureTail
// (internal/engine/task_commit.go) auto-attach fills CaptureTail, never
// Detail, for every Fail/Block with a retained capture, so an identical
// CaptureTail (a caller who also passed the same tail explicitly via
// DetailTail) collapses to one line, not a duplicate.
func effectiveDetailAndTail(p core.Problem) (detail, tail string) {
	switch {
	case p.Detail == "":
		return p.CaptureTail, ""
	case p.CaptureTail == "" || p.CaptureTail == p.Detail:
		return p.Detail, ""
	default:
		return p.Detail, p.CaptureTail
	}
}

// writeCaptureTailLines renders tail's lines as continuation rows
// under a just-written Detail block, matching writeProblemDetailBlock's own
// continuation indent so the tail reads as more captured output for the
// same problem rather than a new one.
func writeCaptureTailLines(b *strings.Builder, tail, indent string, emphasize bool, s Style) {
	for _, line := range splitPresentationLines(tail) {
		fmt.Fprintf(b, "%s%s%s\n", indent, problemDetailIndent, s.emphasized(line, emphasize))
	}
}

// writeProblemDetailBlock renders Detail as an indented multi-line block under
// the evidence connector. When summary is non-empty it opens the block;
// continuations (and all detail lines when summary is empty) are indented
// under it.
func writeProblemDetailBlock(b *strings.Builder, summary, detail, indent string, emphasize bool, s Style) {
	lines := splitPresentationLines(detail)
	if summary == "" {
		if len(lines) == 0 {
			return
		}
		summary, lines = lines[0], lines[1:]
	}
	fmt.Fprintf(b, "%s%s%s %s\n", indent, problemTreeIndent, s.evidenceGlyph(), s.emphasized(summary, emphasize))
	for _, line := range lines {
		fmt.Fprintf(b, "%s%s%s\n", indent, problemDetailIndent, s.emphasized(line, emphasize))
	}
}

// writeProblemDetailLines continues Detail under a subject (├─) row with │ prefixes.
func writeProblemDetailLines(b *strings.Builder, detail, indent string, emphasize bool, s Style) {
	pipe := s.dim("│")
	for _, line := range splitPresentationLines(detail) {
		fmt.Fprintf(b, "%s%s%s %s\n", indent, problemTreeIndent, pipe, s.emphasized(line, emphasize))
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
func writeAction(b *strings.Builder, a core.Action, s Style) {
	glyph := txt.StyleGlyph(txt.GlyphNextAction.Render(s.Profile), txt.SGRCyan, s.Color)
	if a.Command != nil {
		cmd := a.Command.Executable + " " + strings.Join(a.Command.Args, " ")
		fmt.Fprintf(b, "%s  %s\n", glyph, s.paint(cmd, txt.SGRCyan))
		return
	}
	if a.Label != "" {
		fmt.Fprintf(b, "%s  %s\n", glyph, a.Label)
	}
}
