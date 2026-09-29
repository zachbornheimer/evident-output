package engine

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// ProblemSeverity is the closed set of Problem severities.
type ProblemSeverity string

const (
	// SeverityError is the default Problem severity: the owning Task settles
	// Failed if the Define callback otherwise returns nil.
	SeverityError ProblemSeverity = "error"
	// SeverityWarning annotates the Task without failing it. The run is
	// warned and still exits 0 when nothing else failed.
	SeverityWarning ProblemSeverity = "warning"
)

// ProblemOption configures a problem constructed by Block/Fail/Problem helpers.
type ProblemOption interface {
	applyProblem(*Problem)
}

type problemOptionFunc func(*Problem)

func (f problemOptionFunc) applyProblem(p *Problem) { f(p) }

// Severity sets a Problem's severity. The empty value is treated as
// SeverityError at the call site. Any other value is rejected there with a
// context-bearing error.
func Severity(value ProblemSeverity) ProblemOption {
	return problemOptionFunc(func(p *Problem) { p.Severity = string(value) })
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
	p := Problem{Summary: summary, Severity: string(SeverityError)}
	for _, opt := range opts {
		if opt != nil {
			opt.applyProblem(&p)
		}
	}
	if p.Severity == "" {
		p.Severity = string(SeverityError)
	}
	// Single CSI/control neutralization boundary for every construction path.
	return core.SanitizeProblem(p)
}

func classifiedProblemSeverity(p Problem) (ProblemSeverity, error) {
	switch ProblemSeverity(p.Severity) {
	case SeverityError, SeverityWarning:
		return ProblemSeverity(p.Severity), nil
	default:
		return "", fmt.Errorf("evo: invalid problem severity %q: want %q or %q", p.Severity, SeverityError, SeverityWarning)
	}
}
