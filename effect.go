package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/engine"
)

// EffectVerb is the closed set of imperative verbs an opaque Effect may
// declare. There is no write verb: file state goes through File.
type EffectVerb = engine.EffectVerb

// The complete EffectVerb enum.
const (
	EffectAdd       = engine.EffectAdd
	EffectCreate    = engine.EffectCreate
	EffectDelete    = engine.EffectDelete
	EffectInstall   = engine.EffectInstall
	EffectPush      = engine.EffectPush
	EffectRemove    = engine.EffectRemove
	EffectUninstall = engine.EffectUninstall
	EffectUpdate    = engine.EffectUpdate
)

// EffectRecord is one semantic change or plan row.
type EffectRecord = core.EffectRecord

// EffectSpec describes one aggregate opaque mutation Evo cannot model as
// desired state. Constructing an EffectSpec performs no I/O.
type EffectSpec = engine.EffectSpec

// Effect performs one opaque mutation Evo cannot model declaratively — a
// Git ref deletion, a worktree removal, a remote push, an API-side change.
// File state uses File instead. ctx must come from a Task's Define
// callback; called any other way it returns ErrNoTaskContext or
// ErrTaskClosed.
//
// A dry run records one planned Effect and never invokes fn. An apply run
// invokes fn with ctx (the scheduler-owned context) and records the changed
// Effect only when fn returns nil; fn's error is returned unchanged.
//
// When fn committed part of the aggregate before failing, it returns
// PartialEffect(committed, err): Effect records the committed subset as
// changed and still returns err, so the Task fails over a truthful ledger.
//
// A callback that resolves its own Task as anything but Done (Skipped,
// Fail, Block) disowns the work: nothing reaches the ledger. If one Define
// runs several Effects concurrently, that denial disowns every Effect in
// flight at that moment.
//
// When spec.Resource is set, Effect holds it for writing while fn runs,
// waiting out any overlapping claim first. The claim is process-local; fn
// receives the holding context, so tracked work inside fn that needs a
// second resource fails with ErrNestedResourceAcquisition.
func Effect(ctx context.Context, spec EffectSpec, fn func(context.Context) error) error {
	return engine.Effect(ctx, spec, fn)
}

// Effect usage errors (content-free or zero-quantity Effects).
var (
	ErrEffectVerbInvalid         = engine.ErrEffectVerbInvalid
	ErrEffectObjectMissing       = engine.ErrEffectObjectMissing
	ErrEffectQuantityNotPositive = engine.ErrEffectQuantityNotPositive
	ErrEffectCallbackMissing     = engine.ErrEffectCallbackMissing
	ErrEffectMeasuredNegative    = engine.ErrEffectMeasuredNegative
)

// MeasuredEffect is Effect for work whose size is only known once it ran (a
// prune that finds out how many branches it deleted): fn returns the
// quantity it really affected. EffectSpec.Quantity stays the plan, which a
// dry run shows since fn never runs. The changed ledger row and the closing
// summary record the measured quantity, and the JSON document keeps the
// Effect with that final quantity. A returned 0 records no Effect; a
// negative one fails with ErrEffectMeasuredNegative. Errors and
// PartialEffect behave as in Effect.
func MeasuredEffect(ctx context.Context, spec EffectSpec, fn func(context.Context) (int, error)) error {
	return engine.MeasuredEffect(ctx, spec, fn)
}

// PartialEffect is the error an Effect callback returns when committed of
// the requested EffectSpec.Quantity really happened before err stopped the
// rest. Effect records one changed Effect with the spec's Verb and Object
// and Quantity=committed (none when committed is 0), then returns an error
// that keeps err reachable through errors.Is and errors.As, so the Task
// fails. err must be non-nil and 0 <= committed <= spec.Quantity; anything
// else makes Effect return ErrInvalidPartialEffect and record nothing.
// Dry runs never invoke the callback, so PartialEffect cannot arise there.
// A PartialEffect counts once, for the innermost Effect whose callback
// returned it: an outer Effect that passes that error up records nothing
// for it.
// It is not a retry protocol and implies no rollback.
func PartialEffect(committed int, err error) error {
	return engine.PartialEffect(committed, err)
}

// ErrInvalidPartialEffect reports a PartialEffect with a nil cause, a
// negative committed count, or more committed than the Effect requested.
var ErrInvalidPartialEffect = engine.ErrInvalidPartialEffect
