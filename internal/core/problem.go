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
	// EvidenceTail is a raw evidence tail (typically a capture ring via
	// DetailTail) attached alongside an explicit Detail. When Detail is also
	// set, both render — Detail first, EvidenceTail as an additional evidence
	// line underneath — so an explicit Detail is never silently discarded by
	// an auto-attached or explicitly requested evidence tail (or vice versa).
	// When Detail is empty, EvidenceTail alone renders as the problem's detail
	// body (DetailTail's original, still-supported shape).
	EvidenceTail string
	Severity     string
	Count        int64
	Unit         string
	Location     *SourceLocation
	Evidence     []Attachment
	Actions      []Action
	Fields       []Field
	Cause        error
	Sensitive    bool
}

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
// Named Attachment (not Evidence) because Evidence names the retained
// process-output sink — this is a single labeled fact attached to a
// Problem, a different concept from that sink.
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
	p.EvidenceTail = text.Block(p.EvidenceTail)
	p.Subject = text.Text(p.Subject)
	p.Code = text.Text(p.Code)
	p.Unit = text.Text(p.Unit)
	p.Severity = text.Text(p.Severity)
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
