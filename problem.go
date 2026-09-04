package evo

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/sanitize"
)

// Problem is structured evidence explaining a negative item or task outcome.
type Problem struct {
	Code      string
	Subject   string
	Summary   string
	Detail    string
	Severity  string
	Count     int64
	Unit      string
	Location  *SourceLocation
	Evidence  []Attachment
	Actions   []Action
	Fields    []Field
	Cause     error
	Sensitive bool
}

// SourceLocation is a path-based source position. Named SourceLocation
// (not Location) so the Location(...) ProblemOption constructor below can
// keep that name without colliding with its own return type.
type SourceLocation struct {
	Path   string
	Line   int
	Column int
}

// Attachment is an additional label/value problem attachment.
//
// Named Attachment (not Evidence) because Evidence names the retained
// process-output sink (see Evidence in capture.go) — this is a single
// labeled fact attached to a Problem, a different concept from that sink.
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

// ProblemOption configures a problem constructed by Block/Warn/Fail helpers.
type ProblemOption interface {
	applyProblem(*Problem)
}

type problemOptionFunc func(*Problem)

func (f problemOptionFunc) applyProblem(p *Problem) { f(p) }

// Detail sets user-visible detail text (strings only).
func Detail(text string) ProblemOption {
	return problemOptionFunc(func(p *Problem) { p.Detail = text })
}

// Cause attaches a diagnostic error to a Problem.
//
// Deprecated: Use Failf/Blockf's trailing %w instead. Will be removed in v1.0.
func Cause(err error) ProblemOption {
	return problemOptionFunc(func(p *Problem) { p.Cause = err })
}

// Code sets a stable problem code.
func Code(value string) ProblemOption {
	return problemOptionFunc(func(p *Problem) { p.Code = value })
}

// On sets the problem subject.
func On(subject string) ProblemOption {
	return problemOptionFunc(func(p *Problem) { p.Subject = subject })
}

// Count sets a quantity and optional unit.
func Count(value int64, unit ...string) ProblemOption {
	return problemOptionFunc(func(p *Problem) {
		p.Count = value
		if len(unit) > 0 {
			p.Unit = unit[0]
		}
	})
}

// Location sets a source location on a Problem (renamed from At — C5: a
// free-function At collided in name, though not in call syntax, with
// Output.At(visibility), confusing autocomplete and readers alike).
func Location(path string, line, column int) ProblemOption {
	return problemOptionFunc(func(p *Problem) {
		p.Location = &SourceLocation{Path: path, Line: line, Column: column}
	})
}

// Next attaches actions to a problem.
func Next(action Action) ProblemOption {
	return problemOptionFunc(func(p *Problem) {
		p.Actions = append(p.Actions, action)
	})
}

// NextCommand attaches a recommended command action.
func NextCommand(executable string, args ...string) ProblemOption {
	return Next(Command(executable, args...))
}

// splitWrappedMessage separates a Failf/Blockf error into the summary shown
// as the row's headline and the evidence line rendered underneath it. format
// is the caller's original fmt.Errorf format string (before substitution);
// err is fmt.Errorf(format, args...).
//
// A trailing ": %w" or ", %w" in format marks the wrapped error as evidence
// separable from the summary: summary is the text before the separator,
// evidence is the wrapped error's own text. Without a trailing %w — or when
// %w appears elsewhere in format — the whole formatted text is the summary
// and the wrapped error (if any) still feeds evidence.
func splitWrappedMessage(format string, err error) (summary, evidence string) {
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

// formatWarnArgs splits args into printf format arguments and
// ProblemOptions, mirroring formatEntityName/formatReasonName's mixed-args
// extraction (C6: Warn's summary is a printf format when fmt args are
// present; evo.Detail(...) and other ProblemOptions may be mixed into args
// in any position and still apply).
func formatWarnArgs(summary string, args []any) (string, []ProblemOption) {
	if len(args) == 0 {
		return summary, nil
	}
	var opts []ProblemOption
	var fmtArgs []any
	for _, a := range args {
		if opt, ok := a.(ProblemOption); ok {
			opts = append(opts, opt)
			continue
		}
		fmtArgs = append(fmtArgs, a)
	}
	if len(fmtArgs) == 0 {
		return summary, opts
	}
	return fmt.Sprintf(summary, fmtArgs...), opts
}

func applyProblemOptions(summary string, opts []ProblemOption) Problem {
	p := Problem{Summary: summary}
	for _, opt := range opts {
		if opt != nil {
			opt.applyProblem(&p)
		}
	}
	// Single CSI/control neutralization boundary for every construction path.
	return sanitizeProblem(p)
}

// sanitizeProblem neutralizes CSI/control sequences in all human-visible fields.
// Item, Task, and any future entity store problems only through this helper so
// presentation paths cannot diverge on terminal safety (SEC-001).
//
// Detail uses sanitize.Block so multi-line evidence (diffs, capture tails) keeps
// newlines for the flat renderer (P3); other single-line fields still collapse
// newlines to spaces via sanitize.Text.
func sanitizeProblem(p Problem) Problem {
	p.Summary = sanitize.Text(p.Summary)
	p.Detail = sanitize.Block(p.Detail)
	p.Subject = sanitize.Text(p.Subject)
	p.Code = sanitize.Text(p.Code)
	p.Unit = sanitize.Text(p.Unit)
	p.Severity = sanitize.Text(p.Severity)
	if p.Location != nil {
		loc := *p.Location
		loc.Path = sanitize.Text(loc.Path)
		p.Location = &loc
	}
	if len(p.Evidence) > 0 {
		ev := make([]Attachment, len(p.Evidence))
		for i, e := range p.Evidence {
			ev[i] = Attachment{
				Label: sanitize.Text(e.Label),
				Value: sanitize.Text(e.Value),
			}
		}
		p.Evidence = ev
	}
	if len(p.Fields) > 0 {
		fs := make([]Field, len(p.Fields))
		copy(fs, p.Fields)
		for i := range fs {
			fs[i].Key = sanitize.Text(fs[i].Key)
			if s, ok := fs[i].Value.(string); ok {
				fs[i].Value = sanitize.Text(s)
			}
		}
		p.Fields = fs
	}
	return p
}

// storeProblems clones and sanitizes problems for durable entity state.
// Prefer this over cloneProblems alone when assigning to Item/Task state.
func storeProblems(in []Problem) []Problem {
	if len(in) == 0 {
		return nil
	}
	out := cloneProblems(in)
	for i := range out {
		out[i] = sanitizeProblem(out[i])
	}
	return out
}

func cloneProblems(in []Problem) []Problem {
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
