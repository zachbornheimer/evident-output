package record

import "fmt"

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

// WithSeverity sets a Problem's severity. The empty value is treated as
// SeverityError at the call site. Any other value is rejected there with a
// context-bearing error.
func WithSeverity(value ProblemSeverity) ProblemOption {
	return problemOptionFunc(func(p *Problem) { p.Severity = string(value) })
}

// WithDetail sets user-visible detail text (strings only).
func WithDetail(text string) ProblemOption {
	return problemOptionFunc(func(p *Problem) { p.Detail = text })
}

// WithCode sets a stable problem code.
func WithCode(value string) ProblemOption {
	return problemOptionFunc(func(p *Problem) { p.Code = value })
}

// OnSubject sets the problem subject.
func OnSubject(subject string) ProblemOption {
	return problemOptionFunc(func(p *Problem) { p.Subject = subject })
}

// WithCount sets a quantity and optional unit.
func WithCount(value int64, unit ...string) ProblemOption {
	return problemOptionFunc(func(p *Problem) {
		p.Count = value
		if len(unit) > 0 {
			p.Unit = unit[0]
		}
	})
}

// AtLocation sets a source location on a Problem (renamed from At — C5: a
// free-function At collided in name, though not in call syntax, with
// Output.At(visibility), confusing autocomplete and readers alike).
func AtLocation(path string, line, column int) ProblemOption {
	return problemOptionFunc(func(p *Problem) {
		p.Location = &SourceLocation{Path: path, Line: line, Column: column}
	})
}

// WithAction attaches actions to a problem.
func WithAction(action Action) ProblemOption {
	return problemOptionFunc(func(p *Problem) {
		p.Actions = append(p.Actions, action)
	})
}

// WithEvidenceTail attaches captured child-process output as the Problem's
// evidence tail, rendered beneath any explicit Detail. tail is read when the
// option is applied, not when it is built, so output captured in between is
// included; empty text attaches nothing.
func WithEvidenceTail(tail func() string) ProblemOption {
	return problemOptionFunc(func(p *Problem) {
		if text := tail(); text != "" {
			p.EvidenceTail = text
		}
	})
}

// WithCommand attaches a recommended command action.
func WithCommand(executable string, args ...string) ProblemOption {
	return WithAction(Command(executable, args...))
}

// ApplyProblemOptions is the Problem summary and opts describe: severity
// defaults to SeverityError, and every human-visible field is neutralized
// here, once, for every construction path.
func ApplyProblemOptions(summary string, opts []ProblemOption) Problem {
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
	return SanitizeProblem(p)
}

// ClassifiedProblemSeverity is p's severity, or an error naming the invalid
// value when it is neither SeverityError nor SeverityWarning.
func ClassifiedProblemSeverity(p Problem) (ProblemSeverity, error) {
	switch ProblemSeverity(p.Severity) {
	case SeverityError, SeverityWarning:
		return ProblemSeverity(p.Severity), nil
	default:
		return "", fmt.Errorf("evo: invalid problem severity %q: want %q or %q", p.Severity, SeverityError, SeverityWarning)
	}
}
