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
	ProblemOption   = record.ProblemOption
	problemSeverity = record.ProblemSeverity
	reasonOption    = record.ReasonOption
	taxonomyReason  = record.TaxonomyReason
)

const (
	severityError   = record.SeverityError
	severityWarning = record.SeverityWarning
)

// Command builds an action with an executable and arguments.
func actionCommand(executable string, args ...string) Action {
	return asEvoAction(record.Command(executable, args...))
}

// Label builds a plain-text recommended next step with no executable command.
func label(text string) Action { return asEvoAction(record.Label(text)) }

// Severity sets a Problem's severity.
func severity(value problemSeverity) ProblemOption { return record.WithSeverity(value) }

// Detail sets user-visible detail text.
func Detail(text string) ProblemOption { return record.WithDetail(text) }

// Code sets a stable problem code.
func Code(value string) ProblemOption { return record.WithCode(value) }

// On sets the problem subject.
func On(subject string) ProblemOption { return record.OnSubject(subject) }

// Count sets a quantity and optional unit.
func count(value int64, unit ...string) ProblemOption { return record.WithCount(value, unit...) }

// Location sets a source location on a Problem.
func location(path string, line, column int) ProblemOption {
	return record.AtLocation(path, line, column)
}

// Next attaches an action to a problem.
func Next(action Action) ProblemOption { return record.WithAction(asRecordAction(action)) }

// NextCommand attaches a recommended command action.
func nextCommand(executable string, args ...string) ProblemOption {
	return record.WithCommand(executable, args...)
}

// ForSkip restricts a reason to Task.Skipped.
func forSkip() reasonOption { return record.ForSkip() }

// OnTask restricts a reason to the named task.
func onTask(taskName string) reasonOption { return record.OnTask(taskName) }

// reasonGetOrCreate is the Reason registered under name on this run.
func (o *Run) reasonGetOrCreate(name string, opts ...reasonOption) taxonomyReason {
	return o.rec.Reason(name, opts...)
}
