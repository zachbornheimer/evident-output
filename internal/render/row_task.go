package render

import (
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// taskRow is one Task's durable row and everything nested under it. A root
// row, a header-less Group's row and a Group child are the same row and
// differ only in where they sit: prefix comes before the glyph, and nested
// indents the annotations under it.
type taskRow struct {
	t         core.TaskSnapshot
	nameWidth int
	prefix    string
	nested    string
}

// rootRow is t as a row at the run's root (or flattened up to it).
func rootRow(t core.TaskSnapshot, nameWidth int) taskRow {
	return taskRow{t: t, nameWidth: nameWidth, nested: TaskAnnotationIndent}
}

// childRow is t as a row under its Group's header.
func childRow(t core.TaskSnapshot, nameWidth int) taskRow {
	return taskRow{t: t, nameWidth: nameWidth, prefix: GroupChildIndent, nested: problemTreeIndent}
}

// rowHead is what a row's own line settled on: its detail text, the
// Headline the Problems below must not repeat, and which annotation it
// inlined (so the nested block does not repeat that either).
type rowHead struct {
	detail    string
	annotated bool
	Headline  string
	inlined   inlineAnnotation
}

// inlineAnnotation is which one annotation a row carries on its own line.
type inlineAnnotation uint8

const (
	inlineNone inlineAnnotation = iota
	inlineWarning
	inlineFact
	inlineTaxonomy
)

// write renders the row and its nested evidence and annotations.
func (r taskRow) write(b *strings.Builder, s Style) {
	r.t = TaskAtVerbosity(r.t, s.Verbose)
	head, taxonomyVerb := r.head(s)
	r.writeLine(b, head, s)
	r.writeProblems(b, head.Headline, s)
	r.writeNested(b, head.inlined, taxonomyVerb, s)
}

// Headline is the one sentence a row states for its Task: the Task's own
// Summary, or else its first Problem.
func Headline(t core.TaskSnapshot) string {
	if t.Summary != "" || len(t.Problems) == 0 {
		return t.Summary
	}
	return t.Problems[0].Summary
}

// head chooses the row's detail, first match wins: the already-satisfied
// resolution, the Headline, one short inline annotation, the in-flight
// progress/phase, or nothing.
func (r taskRow) head(s Style) (rowHead, Disposition) {
	t := r.t
	if t.Resolution == core.ResolutionAlreadySatisfied {
		return rowHead{detail: AlreadySatisfiedRowDetail(t, s.Color), Headline: alreadySatisfiedDetail}, NoDisposition
	}
	if line := Headline(t); line != "" {
		return rowHead{detail: headlineDetail(t, line, s), Headline: line}, NoDisposition
	}
	if msg, ok := inlineTaskWarning(t); ok {
		return rowHead{detail: inlineWarningText(msg, s), annotated: true, inlined: inlineWarning}, NoDisposition
	}
	if text, verb, ok := inlineTaskTaxonomy(t); ok {
		return rowHead{detail: inlineTaxonomyText(text, verb, s), annotated: true, inlined: inlineTaxonomy}, verb
	}
	if f, ok := inlineTaskFact(t); ok {
		return rowHead{detail: inlineFactText(f, s), annotated: true, inlined: inlineFact}, NoDisposition
	}
	if t.State == core.Running {
		return rowHead{detail: runningTaskDetail(t)}, NoDisposition
	}
	return rowHead{}, NoDisposition
}

// headlineDetail renders a row's Headline. A Failed or Blocked Headline is
// the evidence the reader most needs, so it keeps full intensity, and a
// Task that failed mid-loop keeps the count it reached ("how far did it
// get"). Every other outcome's Headline is subordinate and dims.
func headlineDetail(t core.TaskSnapshot, line string, s Style) string {
	switch t.State {
	case core.Failed:
		if count := ProgressCountText(t.Progress); count != "" {
			return count + "  " + line
		}
		return line
	case core.Blocked:
		return line
	default:
		return s.Dim(line)
	}
}

// writeLine writes the row's own line. The annotation column carries
// taskNameColumnMargin; a row with no detail is trimmed so it never ends in
// dangling whitespace.
func (r taskRow) writeLine(b *strings.Builder, head rowHead, s Style) {
	width := r.nameWidth
	if head.annotated {
		width += taskNameColumnMargin
	}
	unit := DisplayUnit{Glyph: s.stateGlyph(r.t.State), Name: txt.PadRight(r.t.Name, width), Detail: head.detail}
	b.WriteString(unit.Render(r.prefix))
	b.WriteByte('\n')
}

// writeProblems writes the Task's Problems under its row, at most
// maxVisibleProblems of them plus an "and N more failures" line. A Problem
// that only repeats the row's Headline is dropped, and one whose Detail
// carries the evidence loses its repeated summary.
func (r taskRow) writeProblems(b *strings.Builder, rowHeadline string, s Style) {
	emphasize := r.t.State == core.Failed || r.t.State == core.Blocked
	problems := r.t.Problems
	omitted := max(len(problems)-maxVisibleProblems, 0)
	problems = problems[:len(problems)-omitted]
	for _, p := range problems {
		p = dedupeCaptureTailAgainstRow(p, rowHeadline)
		repeatsRow := p.Summary != "" && p.Summary == rowHeadline
		if repeatsRow && p.Detail == "" && p.CaptureTail == "" && p.Subject == "" {
			continue
		}
		if repeatsRow && (p.Detail != "" || p.CaptureTail != "") {
			p.Summary = ""
		}
		writeProblem(b, p, r.prefix, emphasize, s)
	}
	if omitted > 0 {
		writeProblem(b, core.Problem{
			Summary: fmt.Sprintf("and %d more failures", omitted),
			Count:   int64(omitted),
			Unit:    "failures",
		}, r.prefix, emphasize, s)
	}
}

// writeNested writes the row's tallies, verification details, warnings and
// facts, leaving out whichever annotation the row's own line inlined.
func (r taskRow) writeNested(b *strings.Builder, inlined inlineAnnotation, taxonomyVerb Disposition, s Style) {
	t := r.t
	warnings, facts := t.Warnings, t.Facts
	switch inlined {
	case inlineWarning:
		warnings = nil
	case inlineFact:
		facts = nil
	}
	WriteDispositions(b, r.nested, TaskDispositions(t), taxonomyVerb, s)
	WriteVerificationDetails(b, t.Verification, r.nested, t.State == core.Failed, s)
	WriteNestedTaskWarnings(b, warnings, r.nested, s)
	writeNestedTaskFacts(b, facts, r.nested, s)
}
