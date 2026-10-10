package engine

import "github.com/zachbornheimer/evident-output/internal/record"

// Everything below forwards to internal/record, which now owns the recording
// verbs. engine keeps the names so the root package needs no import change
// yet; slice 8 deletes this file and repoints the root at record directly.

type (
	Failure         = record.Failure
	LogRecord       = record.LogRecord
	ProblemOption   = record.ProblemOption
	ProblemSeverity = record.ProblemSeverity
	ReasonOption    = record.ReasonOption
	TaxonomyReason  = record.TaxonomyReason
)

const (
	SeverityError   = record.SeverityError
	SeverityWarning = record.SeverityWarning
)

// Command builds an action with an executable and arguments.
func Command(executable string, args ...string) Action { return record.Command(executable, args...) }

// Label builds a plain-text recommended next step with no executable command.
func Label(text string) Action { return record.Label(text) }

// Severity sets a Problem's severity.
func Severity(value ProblemSeverity) ProblemOption { return record.WithSeverity(value) }

// Detail sets user-visible detail text.
func Detail(text string) ProblemOption { return record.WithDetail(text) }

// Code sets a stable problem code.
func Code(value string) ProblemOption { return record.WithCode(value) }

// On sets the problem subject.
func On(subject string) ProblemOption { return record.OnSubject(subject) }

// Count sets a quantity and optional unit.
func Count(value int64, unit ...string) ProblemOption { return record.WithCount(value, unit...) }

// Location sets a source location on a Problem.
func Location(path string, line, column int) ProblemOption {
	return record.AtLocation(path, line, column)
}

// Next attaches an action to a problem.
func Next(action Action) ProblemOption { return record.WithAction(action) }

// NextCommand attaches a recommended command action.
func NextCommand(executable string, args ...string) ProblemOption {
	return record.WithCommand(executable, args...)
}

// ForSkip restricts a reason to TaskHandle.Skipped.
func ForSkip() ReasonOption { return record.ForSkip() }

// OnTask restricts a reason to the named task.
func OnTask(taskName string) ReasonOption { return record.OnTask(taskName) }

// reasonGetOrCreate is the Reason registered under name on this run.
func (o *Output) reasonGetOrCreate(name string, opts ...ReasonOption) TaxonomyReason {
	return o.rec.Reason(name, opts...)
}
