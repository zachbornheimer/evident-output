package evo

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Fact records a discovered name/value annotation on the default instance's
// run itself, not on any one task — evo.Fact's package-level form. See
// Output.Fact.
func Fact(name, value string) {
	Default().Fact(name, value)
}

// Fact accumulates a run-scoped discovered name/value annotation (P8
// symmetry with TaskHandle.Fact) — information about the run, fire-and-
// forget, rendered as a durable dim "name  value" line. A nil Output is
// safe and records nothing.
func (o *Output) Fact(name, value string) {
	if o == nil {
		return
	}
	f := core.SanitizeFact(FactRecord{Name: txt.Text(name), Value: txt.Text(value)})
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	o.runFacts = append(o.runFacts, f)
	o.bumpLocked()
}

// Warn records a run-scoped warning on the default instance — evo.Warn's
// package-level form. See Output.Warn.
func Warn(summary string, args ...any) {
	Default().Warn(summary, args...)
}

// Warn accumulates a run-scoped warning annotation (P8 symmetry with
// TaskHandle.Warn) — a warning about the run itself, not about any one
// task. Feeds the conclusion's "· warned" band exactly like a task warning,
// never a headline of its own (evo-rec.md "warnings annotate lifecycle;
// they do not replace it"). summary is a printf format when fmt args are
// present, matching TaskHandle.Warn's C6 shape. A nil Output is safe and
// records nothing.
func (o *Output) Warn(summary string, args ...any) {
	if o == nil {
		return
	}
	formatted, opts := formatWarnArgs(summary, args)
	p := applyProblemOptions(txt.Text(formatted), opts)
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	o.runWarnings = append(o.runWarnings, p)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "run.warned", OutputID: o.outputID})
}
