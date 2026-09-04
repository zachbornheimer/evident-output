package evo

import txt "github.com/zachbornheimer/evident-output/internal/text"

// planLedger is the internal handle for one task's planned (dry-run)
// effects — the section named after the task that TaskHandle's mutation
// verbs (task_mutations.go) record planned effects into during DryRun.
// Unexported: P1/P13 removed the caller-facing Output.Plan entry point and
// its builder methods from the public surface — PlanSnapshot stays the
// public, read-only view.
type planLedger struct {
	out *Output
	id  string
}

// record records a planned verb/quantity/object. A zero quantity records no
// row but still remembers verb as the section's intended verb — see
// changeLedger.record for the empty-section rationale (evo-rec.md
// guess-driven default #1).
func (p *planLedger) record(verb string, quantity int, object string) *planLedger {
	p.out.mu.Lock()
	defer p.out.mu.Unlock()
	st := p.find()
	if st == nil {
		return p
	}
	if err := p.out.ensureOpen(); err != nil {
		p.out.recordMisuse(err)
		return p
	}
	verb = txt.Text(verb)
	if st.intendedVerb == "" {
		st.intendedVerb = verb
	}
	if quantity == 0 {
		return p
	}
	st.records = append(st.records, EffectRecord{
		Verb:     verb,
		Quantity: int64(quantity),
		HasQty:   true,
		Object:   txt.Text(object),
	})
	p.out.bumpLocked()
	p.out.appendEventLocked(Event{Type: "plan.recorded", EntityID: p.id})
	return p
}

func (p *planLedger) recordNoQty(verb, object string) *planLedger {
	p.out.mu.Lock()
	defer p.out.mu.Unlock()
	st := p.find()
	if st == nil {
		return p
	}
	if err := p.out.ensureOpen(); err != nil {
		p.out.recordMisuse(err)
		return p
	}
	verb = txt.Text(verb)
	if st.intendedVerb == "" {
		st.intendedVerb = verb
	}
	st.records = append(st.records, EffectRecord{
		Verb:   verb,
		Object: txt.Text(object),
	})
	p.out.bumpLocked()
	p.out.appendEventLocked(Event{Type: "plan.recorded", EntityID: p.id})
	return p
}

// declareIntendedVerb mirrors changeLedger.declareIntendedVerb for plan
// sections.
func (p *planLedger) declareIntendedVerb(verb string) {
	p.out.mu.Lock()
	defer p.out.mu.Unlock()
	st := p.find()
	if st == nil || st.intendedVerb != "" {
		return
	}
	st.intendedVerb = txt.Text(verb)
}

func (p *planLedger) find() *planState {
	for _, st := range p.out.plans {
		if st.id == p.id {
			return st
		}
	}
	return nil
}
