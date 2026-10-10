package engine

import (
	"github.com/zachbornheimer/evident-output/internal/render"
)

func (o *Output) appendEventLocked(e Event) {
	e.Timestamp = o.cfg.clock.Now()
	e.SchemaVersion = EventSchemaVersion
	if e.OutputID == "" {
		e.OutputID = o.outputID
	}
	e = o.rec.AppendEvent(e, o.cfg.maxEvents)
	if o.cfg.projection == ProjectionStreamJSON {
		o.writeStreamJSONLocked(e)
	}
}

func (o *Output) writeStreamJSONLocked(e Event) {
	w := o.cfg.primary
	if w == nil {
		return
	}
	row, err := render.EncodeEventJSON(e)
	if err != nil {
		return
	}
	_, _ = w.Write(row)
	_, _ = w.Write([]byte{'\n'})
	if f, ok := w.(flusher); ok {
		_ = f.Flush()
	}
}

// Events returns a copy of durable events (v0.1 journal).
func (o *Output) copyEvents() []Event {
	return o.rec.Events(o.cfg.maxEvents)
}
