package project

import (
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// TaskRow is one Task's durable row and everything nested under it. A root
// row, a header-less Group's row and a Group child are the same row and
// differ only in where they sit: prefix comes before the glyph, and nested
// indents the annotations under it.
type TaskRow struct {
	t         record.TaskSnapshot
	nameWidth int
	prefix    string
	nested    string
	// glyphState, when set, is the state the row's glyph shows instead of
	// the Task's: a Group's row that carries its own Task shows the Group's.
	glyphState record.State
}

// RootRow is t as a row at the run's root (or flattened up to it).
func RootRow(t record.TaskSnapshot, nameWidth int) TaskRow {
	return TaskRow{t: t, nameWidth: nameWidth, nested: TaskAnnotationIndent}
}

// ChildRow is t as a row under its Group's header.
func ChildRow(t record.TaskSnapshot, nameWidth int) TaskRow {
	return TaskRow{t: t, nameWidth: nameWidth, prefix: GroupChildIndent, nested: ProblemTreeIndent}
}

// FoldedGroupRow is a Group's row carrying its own Task: the Group's glyph
// and name with the Task's outcome, summary and evidence.
func FoldedGroupRow(own record.TaskSnapshot, group record.State) TaskRow {
	return TaskRow{t: own, glyphState: group, nested: TaskAnnotationIndent}
}

// rowHead is what a row's own line settled on: its detail text, the
// headline the Problems below must not repeat, and which annotation it
// inlined (so the nested block does not repeat that either).
type rowHead struct {
	detail    string
	annotated bool
	headline  string
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

// Write renders the row and its nested evidence and annotations.
func (r TaskRow) Write(b *strings.Builder, s Style) {
	r.t = TaskAtVerbosity(r.t, s.Verbose)
	head, taxonomyVerb := r.head(s)
	r.writeLine(b, head, s)
	r.writeEvidenceTree(b, head.headline, s)
	r.writeNested(b, head.inlined, taxonomyVerb, s)
}

// Headline is the one sentence a row states for its Task: the Task's own
// Summary, or else its first Problem.
func Headline(t record.TaskSnapshot) string {
	if t.Summary != "" || len(t.Problems) == 0 {
		return t.Summary
	}
	return t.Problems[0].Summary
}

// head chooses the row's detail, first match wins: the already-satisfied
// resolution, the headline, one short inline annotation, the in-flight
// progress/phase, or nothing.
func (r TaskRow) head(s Style) (rowHead, Disposition) {
	t := r.t
	if t.State == record.Satisfied {
		return rowHead{detail: AlreadySatisfiedRowDetail(t, s.Color), headline: AlreadySatisfiedDetail}, NoDisposition
	}
	if line := Headline(t); line != "" {
		return rowHead{detail: headlineDetail(t, line, s), headline: line}, NoDisposition
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
	if t.State == record.Running {
		return rowHead{detail: runningTaskDetail(t)}, NoDisposition
	}
	return rowHead{}, NoDisposition
}

// headlineDetail renders a row's headline. A Failed or Blocked headline is
// the evidence the reader most needs, so it keeps full intensity, and a
// Task that failed mid-loop keeps the count it reached ("how far did it
// get"). Every other outcome's headline is subordinate and dims.
func headlineDetail(t record.TaskSnapshot, line string, s Style) string {
	switch t.State {
	case record.Failed:
		if count := ProgressCountText(t.Progress); count != "" {
			return count + "  " + line
		}
		return line
	case record.Refused:
		return line
	default:
		return s.Dim(line)
	}
}

// writeLine writes the row's own line. The annotation column carries
// taskNameColumnMargin; a row with no detail is trimmed so it never ends in
// dangling whitespace.
func (r TaskRow) writeLine(b *strings.Builder, head rowHead, s Style) {
	width := r.nameWidth
	if head.annotated {
		width += taskNameColumnMargin
	}
	state := r.t.State
	if r.glyphState != "" {
		state = r.glyphState
	}
	unit := DisplayUnit{Glyph: s.RowGlyph(state, r.t.Problems), Name: PadRight(r.t.Name, width), Detail: head.detail}
	b.WriteString(unit.Render(r.prefix))
	b.WriteByte('\n')
}

// writeEvidenceTree writes the row's evidence as one tree: its Problems,
// at most maxVisibleProblems of them plus an "and N more failures" line,
// then its verification attributes. Only the tree's last branch closes it.
func (r TaskRow) writeEvidenceTree(b *strings.Builder, rowHeadline string, s Style) {
	details := VisibleVerificationDetails(r.t.Verification, r.t.State == record.Failed, s)
	problems := r.visibleProblems(rowHeadline, details)
	emphasize := r.t.State == record.Failed || r.t.State == record.Refused
	for i, p := range problems {
		writeProblem(b, p, r.prefix, emphasize, len(details) == 0 && i == len(problems)-1, s)
	}
	writeVerificationBranches(b, details, r.prefix, s)
}

// visibleProblems is the Problems the row shows. A Problem that only
// repeats the row's headline is dropped, and one whose Detail or Subject
// carries the evidence loses its repeated summary. A line that restates a
// failed attribute in details is dropped too: that attribute's branch states it.
func (r TaskRow) visibleProblems(rowHeadline string, details []record.VerificationDetail) []record.Problem {
	problems := r.t.Problems
	omitted := max(len(problems)-maxVisibleProblems, 0)
	problems = problems[:len(problems)-omitted]
	var shown []record.Problem
	for _, p := range problems {
		p = dedupeEvidenceTailAgainstRow(p, rowHeadline)
		p = withoutRestatedAttributes(p, details)
		if p.Summary == "" && p.Subject == "" && p.Detail == "" && p.EvidenceTail == "" {
			continue
		}
		repeatsRow := p.Summary != "" && p.Summary == rowHeadline
		if repeatsRow && p.Detail == "" && p.EvidenceTail == "" && (p.Subject == "" || r.t.State == record.Excluded) {
			continue
		}
		if repeatsRow && (p.Detail != "" || p.EvidenceTail != "" || p.Subject != "") {
			p.Summary = ""
		}
		shown = append(shown, p)
	}
	if omitted > 0 {
		shown = append(shown, record.Problem{
			Summary: fmt.Sprintf("and %d more failures", omitted),
			Count:   int64(omitted),
			Unit:    "failures",
		})
	}
	return shown
}

// writeNested writes the row's tallies, warnings and facts, leaving out whichever annotation the row's own line inlined.
func (r TaskRow) writeNested(b *strings.Builder, inlined inlineAnnotation, taxonomyVerb Disposition, s Style) {
	t := r.t
	warnings, facts := t.Warnings, t.Facts
	switch inlined {
	case inlineWarning:
		warnings = nil
	case inlineFact:
		facts = nil
	}
	WriteDispositions(b, r.nested, taskDispositions(t), taxonomyVerb, s)
	WriteNestedTaskWarnings(b, warnings, r.nested, s)
	writeNestedTaskFacts(b, facts, r.nested, s)
}
