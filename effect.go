package evo

import "github.com/zachbornheimer/evident-output/internal/change"

type Verb string

const (
	Create Verb = "create"
	Update Verb = "update"
	Delete Verb = "delete"
)

type EffectSpec struct {
	Verb     Verb
	Object   string
	Quantity int
	Path     string
	// Resource is a logical resource name: two Effects whose Resources are
	// equal after trimming surrounding whitespace never overlap.
	Resource string
	// claim is a resource tests name directly.
	claim resource
}

func (s EffectSpec) toChange() change.EffectSpec {
	v := change.EffectVerb(s.Verb)
	switch s.Verb {
	case Create:
		v = change.EffectCreate
	case Update:
		v = change.EffectUpdate
	case Delete:
		v = change.EffectDelete
	}
	claim := s.claim
	if s.Resource != "" {
		claim = change.LogicalResource(s.Resource)
	}
	return change.EffectSpec{Verb: v, Object: s.Object, Quantity: s.Quantity, Resource: claim}
}
