package engine

import (
	"github.com/zachbornheimer/evident-output/internal/render"
)

// appendEventLocked journals e. The journal is in the order things happened,
// so the record is followed first: a settle a graph goroutine made on its own
// is journaled before an event this section writes after it, however late the
// record's listener hears it.
func (o *Output) appendEventLocked(e Event) {
	o.followRecordLocked()
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

// copyEvents is a copy of the durable events (v0.1 journal), current with the
// record: every settle the graph made is journaled before it is read.
func (o *Output) copyEvents() []Event {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.followRecordLocked()
	return o.rec.Events(o.cfg.maxEvents)
}
