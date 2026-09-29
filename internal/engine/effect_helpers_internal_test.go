package engine

import "context"

// effectOf is a Define callback performing one opaque Effect whose own work
// is a no-op — the shape a test needs to drive the Plan/Changes ledger.
func effectOf(verb EffectVerb, object string, quantity int) func(context.Context) error {
	return func(ctx context.Context) error {
		return Effect(ctx, EffectSpec{Verb: verb, Object: object, Quantity: quantity},
			func(context.Context) error { return nil })
	}
}
