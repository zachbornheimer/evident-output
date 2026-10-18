package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
)

// effectOf is a Define callback performing one opaque Effect whose own work
// is a no-op — the shape a test needs to drive the Plan/Changes ledger.
func effectOf(verb Verb, object string, quantity int) func(context.Context) error {
	return func(ctx context.Context) error {
		return Effect(ctx, EffectSpec{Verb: verb, Object: object, Quantity: quantity},
			func(context.Context) error { return nil })
	}
}

// engineEffectOf is effectOf for a verb the engine conjugates but the public Verb does not offer, so the
// ledger's tense table stays tested for every verb it knows.
func engineEffectOf(verb change.EffectVerb, object string, quantity int) func(context.Context) error {
	return func(ctx context.Context) error {
		return change.Effect(ctx, change.EffectSpec{Verb: verb, Object: object, Quantity: quantity},
			func(context.Context) error { return nil })
	}
}
