package core

import (
	"errors"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/text"
)

// Problem is structured evidence explaining a negative item or task outcome.
type Problem struct {
	Code    string
	Subject string
	Summary string
	Detail  string
	// CaptureTail is a raw capture-ring tail (typically via Capture.DetailTail)
	// attached alongside an explicit Detail. When Detail is also set, both
	// render — Detail first, CaptureTail as an additional line underneath —
	// so an explicit Detail is never silently discarded by an auto-attached
	// or explicitly requested capture tail (or vice versa). When Detail is
	// empty, CaptureTail alone renders as the problem's detail body
	// (DetailTail's original, still-supported shape). Renamed from
	// EvidenceTail (removed in 1.1) in the 1.1 vocabulary freeze (E-121): this field holds
	// retained process output, not satisfaction proof, so it must not share
	// the Evidence name with the Evidence field below. The wire JSON key
	// stays "evidence_tail" (internal/wire/problem.go) — a deliberate,
	// documented wire-compat decision: run.v2 payloads already on disk use
	// that key, and this Go-level rename does not touch the schema.
	CaptureTail string
	// Severity is error (the zero value normalizes to it) or warning. A
	// warning never fails the Task or run it is recorded on.
	Severity  ProblemSeverity
	Count     int64
	Unit      string
	Location  *SourceLocation
	Evidence  []Attachment
	Actions   []Action
	Fields    []Field
	Cause     error
	Sensitive bool
}

// ProblemSeverity says whether a Problem fails the work it is recorded on
// (SeverityError, the default) or only warns (SeverityWarning).
type ProblemSeverity string

const (
	// SeverityError fails the owning Define (or the run) when recorded.
	SeverityError ProblemSeverity = "error"
	// SeverityWarning sets "warned" and never fails anything.
	SeverityWarning ProblemSeverity = "warning"
)

// normalized closes the enum: only SeverityWarning warns; the zero value
// and any unknown spelling are SeverityError, the side that fails loudly.
func (s ProblemSeverity) normalized() ProblemSeverity {
	if s == SeverityWarning {
		return SeverityWarning
	}
	return SeverityError
}

// IsWarning reports whether p only warns rather than fails.
func (p Problem) IsWarning() bool { return p.Severity.normalized() == SeverityWarning }

// SourceLocation is a path-based source position. Named SourceLocation
// (not Location) so the Location(...) ProblemOption constructor can keep
// that name without colliding with its own return type.
type SourceLocation struct {
	Path   string
	Line   int
	Column int
}

// Attachment is an additional label/value problem attachment.
//
// Named Attachment, not Evidence: Evidence is satisfaction proof (Verify)
// and Capture is the retained process-output sink. This is a single
// labeled fact attached to a Problem.
type Attachment struct {
	Label string
	Value string
}

// Field is a structured diagnostic or log field.
type Field struct {
	Key       string
	Value     any
	Sensitive bool
}

// RedactedValue replaces a Sensitive Field's Value everywhere evo renders
// or projects one — human output, JSON, evo.run, and JSONL alike — so the
// literal has one owner instead of a copy hard-coded at each call site.
const RedactedValue = "***"

// SplitWrappedMessage separates a Failf/Blockf error into the summary shown
// as the row's headline and the evidence line rendered underneath it. format
// is the caller's original fmt.Errorf format string (before substitution);
// err is fmt.Errorf(format, args...).
//
// A trailing ": %w" or ", %w" in format marks the wrapped error as evidence
// separable from the summary: summary is the text before the separator,
// evidence is the wrapped error's own text. Without a trailing %w — or when
// %w appears elsewhere in format — the whole formatted text is the summary
// and the wrapped error (if any) still feeds evidence.
func SplitWrappedMessage(format string, err error) (summary, evidence string) {
	full := err.Error()
	wrapped := errors.Unwrap(err)
	if wrapped == nil {
		return full, ""
	}
	evidence = wrapped.Error()
	for _, sep := range [...]string{": %w", ", %w"} {
		if !strings.HasSuffix(format, sep) {
			continue
		}
		head := strings.TrimSuffix(sep, "%w")
		if trimmed, ok := strings.CutSuffix(full, head+evidence); ok {
			return trimmed, evidence
		}
	}
	return full, evidence
}

// SanitizeProblem neutralizes CSI/control sequences in all human-visible
// fields. Item, Task, and any future entity store problems only through
// this helper so presentation paths cannot diverge on terminal safety
// (SEC-001).
//
// Detail uses text.Block so multi-line evidence (diffs, capture tails) keeps
// newlines for the flat renderer (P3); other single-line fields still
// collapse newlines to spaces via text.Text.
func SanitizeProblem(p Problem) Problem {
	p.Summary = text.Text(p.Summary)
	p.Detail = text.Block(p.Detail)
	p.CaptureTail = text.Block(p.CaptureTail)
	p.Subject = text.Text(p.Subject)
	p.Code = text.Text(p.Code)
	p.Unit = text.Text(p.Unit)
	p.Severity = p.Severity.normalized()
	if p.Location != nil {
		loc := *p.Location
		loc.Path = text.Text(loc.Path)
		p.Location = &loc
	}
	if len(p.Evidence) > 0 {
		ev := make([]Attachment, len(p.Evidence))
		for i, e := range p.Evidence {
			ev[i] = Attachment{
				Label: text.Text(e.Label),
				Value: text.Text(e.Value),
			}
		}
		p.Evidence = ev
	}
	if len(p.Fields) > 0 {
		fs := make([]Field, len(p.Fields))
		copy(fs, p.Fields)
		for i := range fs {
			fs[i].Key = text.Text(fs[i].Key)
			if s, ok := fs[i].Value.(string); ok {
				fs[i].Value = text.Text(s)
			}
		}
		p.Fields = fs
	}
	return p
}

// StoreProblems clones and sanitizes problems for durable entity state.
// Prefer this over CloneProblems alone when assigning to Item/Task state.
func StoreProblems(in []Problem) []Problem {
	if len(in) == 0 {
		return nil
	}
	out := CloneProblems(in)
	for i := range out {
		out[i] = SanitizeProblem(out[i])
	}
	return out
}

// CloneProblems deep-copies the mutable slices/pointers a Problem carries.
func CloneProblems(in []Problem) []Problem {
	if len(in) == 0 {
		return nil
	}
	out := make([]Problem, len(in))
	copy(out, in)
	for i := range out {
		if len(out[i].Actions) > 0 {
			out[i].Actions = append([]Action(nil), out[i].Actions...)
		}
		if len(out[i].Fields) > 0 {
			out[i].Fields = append([]Field(nil), out[i].Fields...)
		}
		if len(out[i].Evidence) > 0 {
			out[i].Evidence = append([]Attachment(nil), out[i].Evidence...)
		}
		if out[i].Location != nil {
			loc := *out[i].Location
			out[i].Location = &loc
		}
	}
	return out
}
