package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
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
	o.rec.RecordRunFact(f)
	o.bumpLocked()
	o.emitWireEventLocked(wire.EventFactRecorded, "", wire.ToFactDoc(f).EventPayload())
	o.writeDurableTextLocked(txt.Dim(f.Name+"  "+f.Value, !o.cfg.noColor) + "\n")
}

// warnLocked records p as a run-scoped warning and renders it. Callers
// must already hold o.mu. Public Warn was removed in 1.1; this is the
// engine's own path (a failed manifest flush) onto the same projection
// TaskHandle.Problem(..., Severity(SeverityWarning)) uses for the
// conclusion's warned modifier.
func (o *Output) warnLocked(p Problem) {
	o.rec.RecordRunWarning(p)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "run.warned", OutputID: o.outputID})
	o.emitWireEventLocked(wire.EventWarningRecorded, "", wire.ToProblemDoc(p).EventPayload())
	glyph := txt.StyleGlyph(txt.GlyphWarningState.Render(o.cfg.glyphs), txt.SGRYellow, !o.cfg.noColor)
	o.writeDurableTextLocked(glyph + " " + p.Summary + "\n")
}
