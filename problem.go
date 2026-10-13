package evo

import (
	"github.com/zachbornheimer/evident-output/internal/record"
)

// Problem, ProblemOption and Action live with the record aliases until the slice-9 LAYOUT split.

// --- folded from record_aliases.go ---

// Everything below forwards to internal/record, which now owns the recording
// verbs. engine keeps the names so the root package needs no import change
// yet; slice 8 deletes this file and repoints the root at record directly.

type (
	logRecord       = record.LogRecord
	problemSeverity = record.ProblemSeverity
	reasonOption    = record.ReasonOption
	taxonomyReason  = record.TaxonomyReason
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

// Command builds an action with an executable and arguments.
func actionCommand(executable string, args ...string) Action {
	return asEvoAction(record.Command(executable, args...))
}

// Label builds a plain-text recommended next step with no executable command.
func label(text string) Action { return asEvoAction(record.Label(text)) }

// Severity sets a Problem's severity.
func severity(value problemSeverity) ProblemOption {
	return problemOption{record.WithSeverity(value)}
}

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

// Count sets a quantity and optional unit.
func count(value int64, unit ...string) ProblemOption {
	return problemOption{record.WithCount(value, unit...)}
}

// Location sets a source location on a Problem.
func location(path string, line, column int) ProblemOption {
	return problemOption{record.AtLocation(path, line, column)}
}

// Next attaches an action to a problem.
func Next(action Action) ProblemOption {
	return problemOption{record.WithAction(asRecordAction(action))}
}

// NextCommand attaches a recommended command action.
func nextCommand(executable string, args ...string) ProblemOption {
	return problemOption{record.WithCommand(executable, args...)}
}

// ForSkip restricts a reason to Task.Skipped.
func forSkip() reasonOption { return record.ForSkip() }

// OnTask restricts a reason to the named task.
func onTask(taskName string) reasonOption { return record.OnTask(taskName) }

// reasonGetOrCreate is the Reason registered under name on this run.
func (o *Run) reasonGetOrCreate(name string, opts ...reasonOption) taxonomyReason {
	return o.rec.Reason(name, opts...)
}
