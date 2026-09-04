package evo

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// Problem is structured evidence explaining a negative item or task outcome.
//
// Aliased into internal/core alongside the rest of the data model — see
// Snapshot's doc comment (snapshot.go) for why.
type Problem = core.Problem

// SourceLocation is a path-based source position. Named SourceLocation
// (not Location) so the Location(...) ProblemOption constructor below can
// keep that name without colliding with its own return type.
type SourceLocation = core.SourceLocation

// Attachment is an additional label/value problem attachment.
//
// Named Attachment (not Evidence) because Evidence names the retained
// process-output sink (see Evidence in capture.go) — this is a single
// labeled fact attached to a Problem, a different concept from that sink.
type Attachment = core.Attachment

// Field is a structured diagnostic or log field.
type Field = core.Field

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
	return core.SanitizeProblem(p)
}
