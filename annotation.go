package evo

import (
	txt "github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// redactString applies the configured redactor. It takes no lock: locking Run.mu here deadlocked a
// scheduled failure auto-attaching its evidence tail, where resolve already holds it.
func (r *Run) redactString(s string) string {
	if r == nil {
		return s
	}
	redactor := r.cfg.redactor
	if redactor == nil {
		return s
	}
	return redactor.RedactString(s)
}

// Fact accumulates a run-scoped name/value annotation (like Task.Fact): it renders a durable dim line
// at once and stores it for Snapshot/JSON. A nil Run records nothing.
func (r *Run) Fact(name, value string) *Run {
	if r == nil {
		return r
	}
	f := record.SanitizeFact(factRecord{Name: txt.Text(name), Value: txt.Text(value)})
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpen(); err != nil {
		r.recordMisuse(err)
		return r
	}
	r.graph.Root().Note(f)
	r.bumpLocked()
	r.emitWireEventLocked(wire.EventFactRecorded, "", wire.ToFactDoc(f).EventPayload())
	r.show.Stream.Write(txt.Dim(f.Name+"  "+f.Value, !r.cfg.noColor) + "\n")
	return r
}

// warnLocked records p as a run-scoped warning and renders it; callers hold r.mu.
func (r *Run) warnLocked(p problem) {
	r.graph.Root().Warn(p)
	r.bumpLocked()
	r.appendEventLocked(recEvent{Type: "run.warned", OutputID: r.outputID})
	r.emitWireEventLocked(wire.EventWarningRecorded, "", wire.ToProblemDoc(p).EventPayload())
	glyph := txt.StyleGlyph(txt.GlyphWarningState.Render(r.cfg.glyphs), txt.SGRYellow, !r.cfg.noColor)
	r.show.Stream.Write(glyph + " " + p.Summary + "\n")
}
