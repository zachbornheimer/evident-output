package evo

import "github.com/zachbornheimer/evident-output/internal/sanitize"

// Changes is a handle for durable effects that already occurred.
type Changes struct {
	out *Output
	id  string
}

// Added records an added quantity.
func (c *Changes) Added(quantity int, object string) *Changes {
	return c.Record("added", quantity, object)
}

// Created records a created object.
func (c *Changes) Created(object string) *Changes {
	return c.recordNoQty("created", object)
}

// Updated records an updated quantity.
func (c *Changes) Updated(quantity int, object string) *Changes {
	return c.Record("updated", quantity, object)
}

// Removed records a removed quantity.
func (c *Changes) Removed(quantity int, object string) *Changes {
	return c.Record("removed", quantity, object)
}

// Deleted records a deleted quantity. Part of the verb set unified across
// TaskHandle/Changes/Plan (C10) — TaskHandle/Plan already had Delete;
// Changes was missing its past-tense counterpart.
func (c *Changes) Deleted(quantity int, object string) *Changes {
	return c.Record("deleted", quantity, object)
}

// Wrote records a written object.
func (c *Changes) Wrote(object string) *Changes {
	return c.recordNoQty("wrote", object)
}

// Pushed records a pushed quantity. Part of the verb set unified across
// TaskHandle/Changes/Plan (C10) — Push was previously TaskHandle-only.
func (c *Changes) Pushed(quantity int, object string) *Changes {
	return c.Record("pushed", quantity, object)
}

// RecordName records a verb and one named object without a quantity.
// Quantity is for collapsed counts; RecordName is one named object.
func (c *Changes) RecordName(verb, object string) *Changes {
	return c.recordNoQty(verb, object)
}

// Record records a verb/quantity/object effect. A zero quantity records no
// row (there is nothing to show) but still remembers verb as the section's
// intended verb, so a section that ends up with no rows at all still renders
// "nothing to <verb> <subject>" (evo-rec.md guess-driven default #1: "Zero
// mutations recorded → nothing to delete") instead of inventing a "did 0" row.
func (c *Changes) Record(verb string, quantity int, object string) *Changes {
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
	verb = sanitize.Text(verb)
	if st.intendedVerb == "" {
		st.intendedVerb = verb
	}
	if quantity == 0 {
		return c
	}
	st.records = append(st.records, EffectRecord{
		Verb:     verb,
		Quantity: int64(quantity),
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
	verb = sanitize.Text(verb)
	if st.intendedVerb == "" {
		st.intendedVerb = verb
	}
	st.records = append(st.records, EffectRecord{
		Verb:   verb,
		Object: sanitize.Text(object),
	})
	c.out.bumpLocked()
	c.out.appendEventLocked(Event{Type: "change.recorded", EntityID: c.id})
	return c
}

// declareIntendedVerb records verb as the section's intended verb if none is
// set yet, without adding a row. TaskHandle mutation verbs (task_mutations.go)
// call this with the caller's original imperative verb before Record
// conjugates it to past tense, so an empty section still reads "nothing to
// delete <subject>" rather than "nothing to deleted <subject>".
func (c *Changes) declareIntendedVerb(verb string) {
	c.out.mu.Lock()
	defer c.out.mu.Unlock()
	st := c.find()
	if st == nil || st.intendedVerb != "" {
		return
	}
	st.intendedVerb = sanitize.Text(verb)
}

func (c *Changes) find() *changesState {
	for _, st := range c.out.changes {
		if st.id == c.id {
			return st
		}
	}
	return nil
}
