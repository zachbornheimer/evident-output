package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render"
	txt "github.com/zachbornheimer/evident-output/internal/text"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// Fact records a discovered name/value annotation on the default instance's
// run itself, not on any one task — evo.Fact's package-level form. See
// Output.Fact.
func Fact(name, value string) {
	Default().Fact(name, value)
}

// Fact accumulates a run-scoped discovered name/value annotation (P8
// symmetry with TaskHandle.Fact) — information about the run, fire-and-
// forget: it both renders a durable dim "name  value" line immediately
// (the same "act now" contract Println has) and stores the annotation for
// the structured Snapshot/JSON views. A nil Output is safe and records
// nothing.
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
	o.emitWireEventLocked(wire.EventFactRecorded, "", wire.ToFactDoc(f).EventPayload())
	o.writeDurableTextLocked(txt.Dim(f.Name+"  "+f.Value, !o.cfg.noColor) + "\n")
}

// Problem records one run-scoped Problem: a diagnostic about the run
// itself, not about any one task. A SeverityWarning Problem feeds the
// conclusion's "· warned" band exactly like a task warning and never a
// headline of its own (evo-rec.md "warnings annotate lifecycle; they do
// not replace it"). The default, SeverityError, is a run-level failure,
// the same one Fail records. A nil Output is safe and records nothing.
func (o *Output) Problem(summary string, options ...ProblemOption) {
	if o == nil {
		return
	}
	p := applyProblemOptions(txt.Text(summary), options)
	if !p.IsWarning() {
		o.failWith(p)
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	o.warnLocked(p)
}

// warnLocked records p as a run-scoped warning and renders it. Callers
// must already hold o.mu.
func (o *Output) warnLocked(p Problem) {
	p.Severity = SeverityWarning
	o.runWarnings = append(o.runWarnings, p)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "run.warned", OutputID: o.outputID})
	o.emitWireEventLocked(wire.EventWarningRecorded, "", wire.ToProblemDoc(p).EventPayload())
	o.writeDurableTextLocked(render.WarningLine(o.humanStyle(), p) + "\n")
}
