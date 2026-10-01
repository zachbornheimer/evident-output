package evo

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/engine"
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
// Named Attachment (not Capture) because Capture names the retained
// process-output sink — this is a single labeled fact attached to a
// Problem, a different concept from that sink.
type Attachment = core.Attachment

// Field is a structured diagnostic or log field.
type Field = core.Field

// ProblemOption configures a problem constructed by Block/Fail/Problem helpers.
type ProblemOption = engine.ProblemOption

// ProblemSeverity is the closed set of Problem severities.
type ProblemSeverity = engine.ProblemSeverity

const (
	SeverityError   = engine.SeverityError
	SeverityWarning = engine.SeverityWarning
)

// Severity sets a Problem's severity. Default is SeverityError.
func Severity(value ProblemSeverity) ProblemOption { return engine.Severity(value) }

// Detail sets user-visible detail text (strings only).
func Detail(text string) ProblemOption { return engine.Detail(text) }

// Code sets a stable problem code.
func Code(value string) ProblemOption { return engine.Code(value) }

// On sets the problem subject.
func On(subject string) ProblemOption { return engine.On(subject) }

// Count sets a quantity and optional unit.
func Count(value int64, unit ...string) ProblemOption { return engine.Count(value, unit...) }

// Location sets a source location on a Problem (renamed from At — C5: a
// free-function At collided in name, though not in call syntax, with
// Output.At(visibility), confusing autocomplete and readers alike).
func Location(path string, line, column int) ProblemOption {
	return engine.Location(path, line, column)
}

// Next attaches actions to a problem.
func Next(action Action) ProblemOption { return engine.Next(action) }

// NextCommand attaches a recommended command action.
func NextCommand(executable string, args ...string) ProblemOption {
	return engine.NextCommand(executable, args...)
}

type Failure struct{ inner *engine.Failure }

func (f *Failure) Error() string {
	if f == nil || f.inner == nil {
		return ""
	}
	return f.inner.Error()
}

func (f *Failure) Unwrap() error {
	if f == nil || f.inner == nil {
		return nil
	}
	return f.inner.Unwrap()
}

// Problem codes are stable, machine-readable Problem.Code values a consumer
// matches on instead of parsing Summary text.
const (
	ProblemCodeDuplicateSiblingName    = engine.ProblemCodeDuplicateSiblingName
	ProblemCodeVerificationUnsatisfied = engine.ProblemCodeVerificationUnsatisfied
	ProblemCodeComputedUnordered       = engine.ProblemCodeComputedUnordered
)

// Problem appends one structured diagnostic without resolving the Task.
// Severity defaults to SeverityError: a nil Define return then settles
// Failed. Severity(SeverityWarning) uses the warning projection and never
// fails the Task. An invalid severity is rejected with a context-bearing
// error. Calling it after the Task resolved is misuse, unless an interrupt
// resolved it.
func (t *TaskHandle) Problem(summary string, options ...ProblemOption) *TaskHandle {
	t.impl().Problem(summary, options...)
	return t
}
