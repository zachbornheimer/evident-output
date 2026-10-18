package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
)

// Verb is what an Effect does to its Object. It is a closed set: Create, Update and Delete are the only
// values, no string converts to one, and an EffectSpec with any other Verb, the zero Verb included, is refused.
type Verb uint8

const (
	Create Verb = iota + 1
	Update
	Delete
)

// engine is the engine's spelling of v, empty for a Verb that is none of the three.
func (v Verb) engine() change.EffectVerb {
	switch v {
	case Create:
		return change.EffectCreate
	case Update:
		return change.EffectUpdate
	case Delete:
		return change.EffectDelete
	}
	return ""
}

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
	return change.EffectSpec{Verb: s.Verb.engine(), Object: s.Object, Quantity: s.Quantity, Path: s.Path, Resource: s.Resource}
}

// Effect performs one opaque mutation inside a Task's Define callback; see
// change.Effect.
func Effect(ctx context.Context, spec EffectSpec, fn func(context.Context) error) error {
	return change.Effect(ctx, spec.toChange(), fn)
}

// PartialEffect reports, from an Effect callback, that committed of the
// requested EffectSpec.Quantity really happened before err stopped the rest;
// see change.PartialEffect.
func PartialEffect(committed int, err error) error { return change.PartialEffect(committed, err) }
