package evo

import (
	"errors"
	"fmt"

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
	Location  *Location
	Evidence  []Evidence
	Actions   []Action
	Fields    []Field
	Cause     error
	Sensitive bool
}

// Location is a path-based source position.
type Location struct {
	Path   string
	Line   int
	Column int
}

// Evidence is an additional problem attachment.
type Evidence struct {
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

// Cause attaches a diagnostic error (not shown by default in human output).
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

// At sets a source location.
func At(path string, line, column int) ProblemOption {
	return problemOptionFunc(func(p *Problem) {
		p.Location = &Location{Path: path, Line: line, Column: column}
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

// resolutionError turns a resolved failure/block Problem into the one-line
// error a caller can `return` directly: message is the summary, and a
// Cause(err) option is wrapped with %w so errors.Is/As still reach it. No
// Cause means a plain errors.New(summary) — never a bare nil, so `return
// task.Fail(...)` always hands the caller a real error to propagate.
func resolutionError(p Problem) error {
	if p.Cause != nil {
		return fmt.Errorf("%s: %w", p.Summary, p.Cause)
	}
	return errors.New(p.Summary)
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
		ev := make([]Evidence, len(p.Evidence))
		for i, e := range p.Evidence {
			ev[i] = Evidence{
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
			out[i].Evidence = append([]Evidence(nil), out[i].Evidence...)
		}
		if out[i].Location != nil {
			loc := *out[i].Location
			out[i].Location = &loc
		}
	}
	return out
}
