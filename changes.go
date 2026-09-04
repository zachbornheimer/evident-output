package evo

import txt "github.com/zachbornheimer/evident-output/internal/text"

// changeLedger is the internal handle for one task's durable effects — the
// section named after the task that TaskHandle's mutation verbs
// (task_mutations.go) record committed effects into. Unexported: P1/P13
// removed the caller-facing Output.Changes entry point and its past-tense
// builder methods (Added/Created/Updated/...) from the public surface —
// ChangesSnapshot stays the public, read-only view.
type changeLedger struct {
	out *Output
	id  string
}

// record records a verb/quantity/object effect. A zero quantity records no
// row (there is nothing to show) but still remembers verb as the section's
// intended verb, so a section that ends up with no rows at all still renders
// "nothing to <verb> <subject>" (evo-rec.md guess-driven default #1: "Zero
// mutations recorded → nothing to delete") instead of inventing a "did 0" row.
func (c *changeLedger) record(verb string, quantity int, object string) *changeLedger {
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
	verb = txt.Text(verb)
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
		Object:   txt.Text(object),
	})
	c.out.bumpLocked()
	c.out.appendEventLocked(Event{Type: "change.recorded", EntityID: c.id})
	return c
}

func (c *changeLedger) recordNoQty(verb, object string) *changeLedger {
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
	verb = txt.Text(verb)
	if st.intendedVerb == "" {
		st.intendedVerb = verb
	}
	st.records = append(st.records, EffectRecord{
		Verb:   verb,
		Object: txt.Text(object),
	})
	c.out.bumpLocked()
	c.out.appendEventLocked(Event{Type: "change.recorded", EntityID: c.id})
	return c
}

// declareIntendedVerb records verb as the section's intended verb if none is
// set yet, without adding a row. TaskHandle mutation verbs (task_mutations.go)
// call this with the caller's original imperative verb before record
// conjugates it to past tense, so an empty section still reads "nothing to
// delete <subject>" rather than "nothing to deleted <subject>".
func (c *changeLedger) declareIntendedVerb(verb string) {
	c.out.mu.Lock()
	defer c.out.mu.Unlock()
	st := c.find()
	if st == nil || st.intendedVerb != "" {
		return
	}
	st.intendedVerb = txt.Text(verb)
}

func (c *changeLedger) find() *changesState {
	for _, st := range c.out.changes {
		if st.id == c.id {
			return st
		}
	}
	return nil
}
