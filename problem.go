package evo

import (
	"github.com/zachbornheimer/evident-output/internal/record"
)

type (
	logRecord = record.LogRecord
)

const (
	severityError   = record.SeverityError
	severityWarning = record.SeverityWarning
)

// ProblemOption shapes the Problem that Fail, Refuse, Exclude or
// Task.Problem records. It is sealed: only this package's constructors
// make one.
type ProblemOption interface {
	recordOption() record.ProblemOption
}

// problemOption is every ProblemOption but Detail: one record option.
type problemOption struct{ opt record.ProblemOption }

func (o problemOption) recordOption() record.ProblemOption { return o.opt }

// detail is Detail's option: the longer explanation of a Problem, or a
// context line under a Confirm question.
type detail string

func (d detail) recordOption() record.ProblemOption { return record.WithDetail(string(d)) }

func (d detail) applyConfirm(c *confirmConfig) { c.detail = append(c.detail, string(d)) }

// applyProblemOptions is the Problem summary and opts describe; see
// record.ApplyProblemOptions. A nil option is skipped.
func applyProblemOptions(summary string, opts []ProblemOption) record.Problem {
	recordOpts := make([]record.ProblemOption, 0, len(opts))
	for _, opt := range opts {
		if opt != nil {
			recordOpts = append(recordOpts, opt.recordOption())
		}
	}
	return record.ApplyProblemOptions(summary, recordOpts)
}

// actionCommand builds an action with an executable and arguments.
func actionCommand(executable string, args ...string) Action {
	return asEvoAction(record.Command(executable, args...))
}

// label builds a plain-text recommended next step with no executable command.
func label(text string) Action { return asEvoAction(record.Label(text)) }

// Detail is the longer explanation of a Problem, or a context line under a
// Confirm question.
func Detail(text string) interface {
	ProblemOption
	ConfirmOption
} {
	return detail(text)
}

// Code sets a stable problem code.
func Code(value string) ProblemOption { return problemOption{record.WithCode(value)} }

// On sets the problem subject.
func On(subject string) ProblemOption { return problemOption{record.OnSubject(subject)} }

// Next attaches an action to a problem.
func Next(action Action) ProblemOption {
	return problemOption{record.WithAction(asRecordAction(action))}
}
