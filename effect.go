package evo

// EffectOption configures a mutation-verb call (TaskHandle.Add/Delete/
// Create/Update/Remove/Write/Push). Affected is the only option today.
type EffectOption interface {
	applyEffect(*effectOpts)
}

type effectOpts struct {
	quantity int
	hasQty   bool
}

type effectOptionFunc func(*effectOpts)

func (f effectOptionFunc) applyEffect(o *effectOpts) { f(o) }

// Affected sets how many objects a mutation verb call affected — evo derives
// the ledger's quantity and plural noun from n at render time; the verb
// call's object argument stays a singular noun phrase regardless (see
// TaskHandle.Delete). Without Affected, the mutation records one named
// object with no count (Create/Write's traditional shape): evo owns tense
// and number either way, never the caller.
func Affected(n int) EffectOption {
	return effectOptionFunc(func(o *effectOpts) { o.quantity = n; o.hasQty = true })
}

func applyEffectOptions(opts []EffectOption) effectOpts {
	var o effectOpts
	for _, opt := range opts {
		if opt != nil {
			opt.applyEffect(&o)
		}
	}
	return o
}
