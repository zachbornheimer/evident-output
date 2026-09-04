package evo

import "github.com/zachbornheimer/evident-output/internal/sanitize"

// Plan is a handle for effects that would occur but have not.
type Plan struct {
	out *Output
	id  string
}

// Add records a planned addition.
func (p *Plan) Add(quantity int, object string) *Plan {
	return p.Record("add", quantity, object)
}

// Create records a planned create.
func (p *Plan) Create(object string) *Plan {
	return p.recordNoQty("create", object)
}

// Update records a planned update.
func (p *Plan) Update(quantity int, object string) *Plan {
	return p.Record("update", quantity, object)
}

// Remove records a planned removal.
func (p *Plan) Remove(quantity int, object string) *Plan {
	return p.Record("remove", quantity, object)
}

// Delete records a planned deletion.
func (p *Plan) Delete(quantity int, object string) *Plan {
	return p.Record("delete", quantity, object)
}

// Write records a planned write.
func (p *Plan) Write(object string) *Plan {
	return p.recordNoQty("write", object)
}

// Push records a planned push of quantity of object. Part of the verb set
// unified across TaskHandle/Changes/Plan (C10) — Push was previously
// TaskHandle-only.
func (p *Plan) Push(quantity int, object string) *Plan {
	return p.Record("push", quantity, object)
}

// RecordName records a planned verb and one named object without a quantity.
// Quantity is for collapsed counts; RecordName is one named object.
func (p *Plan) RecordName(verb, object string) *Plan {
	return p.recordNoQty(verb, object)
}

// Record records a planned verb/quantity/object. A zero quantity records no
// row but still remembers verb as the section's intended verb — see
// Changes.Record for the empty-section rationale (evo-rec.md guess-driven
// default #1).
func (p *Plan) Record(verb string, quantity int, object string) *Plan {
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
	verb = sanitize.Text(verb)
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
		Object:   sanitize.Text(object),
	})
	p.out.bumpLocked()
	p.out.appendEventLocked(Event{Type: "plan.recorded", EntityID: p.id})
	return p
}

func (p *Plan) recordNoQty(verb, object string) *Plan {
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
	verb = sanitize.Text(verb)
	if st.intendedVerb == "" {
		st.intendedVerb = verb
	}
	st.records = append(st.records, EffectRecord{
		Verb:   verb,
		Object: sanitize.Text(object),
	})
	p.out.bumpLocked()
	p.out.appendEventLocked(Event{Type: "plan.recorded", EntityID: p.id})
	return p
}

// declareIntendedVerb mirrors Changes.declareIntendedVerb for plan sections.
func (p *Plan) declareIntendedVerb(verb string) {
	p.out.mu.Lock()
	defer p.out.mu.Unlock()
	st := p.find()
	if st == nil || st.intendedVerb != "" {
		return
	}
	st.intendedVerb = sanitize.Text(verb)
}

func (p *Plan) find() *planState {
	for _, st := range p.out.plans {
		if st.id == p.id {
			return st
		}
	}
	return nil
}
