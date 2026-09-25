package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
)

// ProblemOption configures a Problem recorded by Problem, Fail, or Block.
type ProblemOption interface {
	applyProblem(*Problem)
}

type problemOptionFunc func(*Problem)

func (f problemOptionFunc) applyProblem(p *Problem) { f(p) }

// Severity sets whether a Problem fails its work (SeverityError, the
// default) or only warns (SeverityWarning). Fail and Block are outcomes,
// so their Problem is always SeverityError whatever this says.
func Severity(value ProblemSeverity) ProblemOption {
	return problemOptionFunc(func(p *Problem) { p.Severity = value })
}

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

// applyOutcomeProblemOptions is applyProblemOptions for Fail and Block: an
// outcome's Problem is the reason the work failed, so it is always
// SeverityError.
func applyOutcomeProblemOptions(summary string, opts []ProblemOption) Problem {
	p := applyProblemOptions(summary, opts)
	p.Severity = SeverityError
	return p
}
