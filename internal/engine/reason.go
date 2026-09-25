package engine

import (
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// TaxonomyReason names why a task skipped or kept an item. It is the opaque
// handle returned by evo.Reason — duplicate strings merge into one taxonomy
// bucket so a caller can construct one inline at every call site
// (evo.Reason("dirty")) without hand-tracking identity, or lift it to a
// package var once it repeats.
type TaxonomyReason struct {
	name string
}

// Name returns the reason's display label.
func (r TaxonomyReason) Name() string { return r.name }

// reasonGetOrCreate returns the Reason previously registered under name on
// this instance, or registers a new one — the identity backing evo.Reason so
// repeated calls (inline or lifted to a var) merge into one taxonomy bucket.
func (o *Output) reasonGetOrCreate(name string) TaxonomyReason {
	name = txt.Text(name)
	o.mu.Lock()
	defer o.mu.Unlock()
	if r, ok := o.namedReasons[name]; ok {
		return r
	}
	r := TaxonomyReason{name: name}
	if o.namedReasons == nil {
		o.namedReasons = make(map[string]TaxonomyReason)
	}
	o.namedReasons[name] = r
	return r
}
