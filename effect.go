package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
)

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

// Effect performs one opaque mutation inside a Task's Define callback; see
// change.Effect.
func Effect(ctx context.Context, spec EffectSpec, fn func(context.Context) error) error {
	return change.Effect(ctx, spec.toChange(), fn)
}

// PartialEffect reports, from an Effect callback, that committed of the
// requested EffectSpec.Quantity really happened before err stopped the rest;
// see change.PartialEffect.
func PartialEffect(committed int, err error) error { return change.PartialEffect(committed, err) }
