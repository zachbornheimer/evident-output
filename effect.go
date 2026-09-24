package evo

import (
	"context"

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
)
