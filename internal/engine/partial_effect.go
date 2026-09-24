package engine

import (
	"errors"
	"fmt"
	"sync/atomic"
)

// ErrInvalidPartialEffect reports a PartialEffect whose data could not be
// true for the Effect that returned it: a nil cause, a negative committed
// count, or more committed than EffectSpec.Quantity requested. Effect
// records no changed row for it rather than invent a count.
var ErrInvalidPartialEffect = errors.New("evo: invalid PartialEffect")

// partialEffect is the error an Effect callback returns when it committed
// part of its aggregate before failing. It reads as its cause. It belongs
// to the first Effect that reads it: consumed is set then, so an outer
// Effect whose callback passes the same error up never counts the inner
// Effect's commits as its own.
type partialEffect struct {
	committed int
	cause     error
	consumed  atomic.Bool
}

// PartialEffect reports, from an Effect callback, that committed of the
// requested EffectSpec.Quantity really happened before err stopped the
// rest. Effect records the committed subset as changed and still returns
// err, so the Task fails with a truthful ledger. It is not a retry protocol
// and implies no rollback.
func PartialEffect(committed int, err error) error {
	return &partialEffect{committed: committed, cause: err}
}

func (p *partialEffect) Error() string {
	if p.cause == nil {
		return fmt.Sprintf("evo: PartialEffect(%d, nil)", p.committed)
	}
	return p.cause.Error()
}

func (p *partialEffect) Unwrap() error { return p.cause }

// claimPartialEffect consumes and returns the first PartialEffect in err's
// tree (depth first, as errors.As walks it) that no Effect has consumed
// yet, or nil. An inner Effect's consumed one never hides an unconsumed one
// joined after it.
func claimPartialEffect(err error) *partialEffect {
	if p, ok := err.(*partialEffect); ok && !p.consumed.Swap(true) {
		return p
	}
	switch wrapped := err.(type) {
	case interface{ Unwrap() error }:
		return claimPartialEffect(wrapped.Unwrap())
	case interface{ Unwrap() []error }:
		for _, e := range wrapped.Unwrap() {
			if p := claimPartialEffect(e); p != nil {
				return p
			}
		}
	}
	return nil
}

// committedOf reads the committed subset an Effect callback's err claims
// against spec, consuming the PartialEffect. A callback error that is not
// a PartialEffect, or carries one another Effect already consumed,
// committed nothing. An invalid PartialEffect yields ErrInvalidPartialEffect (still
// wrapping the cause) and zero, so no invented count reaches the ledger.
func (s EffectSpec) committedOf(err error) (int, error) {
	p := claimPartialEffect(err)
	if p == nil {
		return 0, err
	}
	switch {
	case p.cause == nil:
		return 0, fmt.Errorf("%w: %s %q: nil cause", ErrInvalidPartialEffect, s.Verb, s.Object)
	case p.committed < 0 || p.committed > s.Quantity:
		return 0, fmt.Errorf("%w: %s %q: committed %d of %d: %w",
			ErrInvalidPartialEffect, s.Verb, s.Object, p.committed, s.Quantity, err)
	}
	return p.committed, err
}
