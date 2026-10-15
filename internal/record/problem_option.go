package record

// ProblemOption configures a problem constructed by Block/Fail/Problem helpers.
type ProblemOption interface {
	applyProblem(*Problem)
}

type problemOptionFunc func(*Problem)

func (f problemOptionFunc) applyProblem(p *Problem) { f(p) }

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

// ApplyProblemOptions is the Problem summary and opts describe, with every
// human-visible field neutralized here, once, for every construction path.
func ApplyProblemOptions(summary string, opts []ProblemOption) Problem {
	p := Problem{Summary: summary}
	for _, opt := range opts {
		if opt != nil {
			opt.applyProblem(&p)
		}
	}
	// Single CSI/control neutralization boundary for every construction path.
	return SanitizeProblem(p)
}
