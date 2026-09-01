package evo

import "github.com/zachbornheimer/evident-output/internal/sanitize"

// Changes is a handle for durable effects that already occurred.
type Changes struct {
	out *Output
	id  string
}

// Added records an added quantity.
func (c *Changes) Added(quantity int64, object string) *Changes {
	return c.Record("added", quantity, object)
}

// Created records a created object.
func (c *Changes) Created(object string) *Changes {
	return c.recordNoQty("created", object)
}

// Updated records an updated quantity.
func (c *Changes) Updated(quantity int64, object string) *Changes {
	return c.Record("updated", quantity, object)
}

// Reused records a reused quantity.
func (c *Changes) Reused(quantity int64, object string) *Changes {
	return c.Record("reused", quantity, object)
}

// Moved records a move.
func (c *Changes) Moved(source, destination string) *Changes {
	return c.recordNoQty("moved", sanitize.Text(source)+" → "+sanitize.Text(destination))
}

// Removed records a removed quantity.
func (c *Changes) Removed(quantity int64, object string) *Changes {
	return c.Record("removed", quantity, object)
}

// Wrote records a written object.
func (c *Changes) Wrote(object string) *Changes {
	return c.recordNoQty("wrote", object)
}

// RecordName records a verb and one named object without a quantity.
// Quantity is for collapsed counts; RecordName is one named object.
func (c *Changes) RecordName(verb, object string) *Changes {
	return c.recordNoQty(verb, object)
}

// Record records a verb/quantity/object effect.
func (c *Changes) Record(verb string, quantity int64, object string) *Changes {
	c.out.mu.Lock()
	defer c.out.mu.Unlock()
	st := c.find()
	if st == nil {
		return c
	}
	if err := c.out.ensureOpen(); err != nil {
		c.out.recordMisuse(err)
		return c
	}
	st.records = append(st.records, EffectRecord{
		Verb:     sanitize.Text(verb),
		Quantity: quantity,
		HasQty:   true,
		Object:   sanitize.Text(object),
	})
	c.out.bumpLocked()
	c.out.appendEventLocked(Event{Type: "change.recorded", EntityID: c.id})
	return c
}

func (c *Changes) recordNoQty(verb, object string) *Changes {
	c.out.mu.Lock()
	defer c.out.mu.Unlock()
	st := c.find()
	if st == nil {
		return c
	}
	if err := c.out.ensureOpen(); err != nil {
		c.out.recordMisuse(err)
		return c
	}
	st.records = append(st.records, EffectRecord{
		Verb:   sanitize.Text(verb),
		Object: sanitize.Text(object),
	})
	c.out.bumpLocked()
	c.out.appendEventLocked(Event{Type: "change.recorded", EntityID: c.id})
	return c
}

func (c *Changes) find() *changesState {
	for _, st := range c.out.changes {
		if st.id == c.id {
			return st
		}
	}
	return nil
}
