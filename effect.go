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
	// Path claims a filesystem path for the callback: Effects whose Paths
	// are equal or one contains the other wait for each other.
	Path string
	// Resource is a logical resource name the callback claims: Effects
	// whose Resources are equal after trimming surrounding whitespace wait
	// for each other. Setting both Path and Resource is ErrInvalidConfig.
	Resource string
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
	return change.EffectSpec{Verb: v, Object: s.Object, Quantity: s.Quantity, Path: s.Path, Resource: s.Resource}
}
