package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Resource names the one shared resource an opaque Effect may claim so
// overlapping work is coordinated (ZYS-840). The unexported marker method
// seals the interface: only this package can produce a Resource, so a
// caller cannot hand Evo an identity the coordination layer does not
// understand. A nil Resource means "no resource claim".
type Resource interface {
	resource()
}

// EffectVerb is the closed set of imperative verbs an opaque Effect may
// declare. There is deliberately no write verb: file state goes through
// File (and Patch), never an opaque callback.
type EffectVerb string

// The complete EffectVerb enum.
const (
	EffectAdd    EffectVerb = "add"
	EffectCreate EffectVerb = "create"
	EffectDelete EffectVerb = "delete"
	EffectPush   EffectVerb = "push"
	EffectRemove EffectVerb = "remove"
	EffectUpdate EffectVerb = "update"
)

// valid reports whether v is one of the declared EffectVerb constants.
func (v EffectVerb) valid() bool {
	switch v {
	case EffectAdd, EffectCreate, EffectDelete, EffectPush, EffectRemove, EffectUpdate:
		return true
	}
	return false
}

// EffectSpec describes one aggregate opaque mutation Evo cannot model as
// desired state (a Git ref deletion, a remote push, an API-side change).
// Constructing an EffectSpec performs no I/O; Effect performs the operation.
type EffectSpec struct {
	// Verb is required and must be one of the EffectVerb constants.
	Verb EffectVerb
	// Object is the singular noun the ledger pluralizes from Quantity
	// ("worktree", "remote ref"). Required.
	Object string
	// Quantity is one aggregate count for this Effect, not N Effects. It
	// must be > 0: when there is nothing to mutate, do not call Effect.
	Quantity int
	// Resource is an optional single resource claim; nil means none.
	Resource Resource
}

// Effect usage errors: a content-free Effect describes no mutation the
// ledger could truthfully record, so Effect refuses it without invoking fn.
var (
	ErrEffectVerbInvalid         = errors.New("evo: EffectSpec.Verb must be one of the EffectVerb constants")
	ErrEffectObjectMissing       = errors.New("evo: EffectSpec.Object is required")
	ErrEffectQuantityNotPositive = errors.New("evo: EffectSpec.Quantity must be > 0")
	ErrEffectCallbackMissing     = errors.New("evo: Effect requires a non-nil callback")
)

// Effect performs one opaque mutation inside a Task's Define callback
// (ctx must come from one; see taskScope). A dry run records one planned
// Effect and never invokes fn. An apply run invokes fn with ctx — the
// scheduler-owned context — and records the changed Effect only when fn
// returns nil; fn's error is returned unchanged so Define can return it.
func Effect(ctx context.Context, spec EffectSpec, fn func(context.Context) error) error {
	task, err := taskScope(ctx)
	if err != nil {
		return err
	}
	if err := spec.validate(fn); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("evo: Effect %s %q: %w", spec.Verb, spec.Object, err)
	}
	if !task.out.DryRun() {
		if err := fn(ctx); err != nil {
			return err
		}
	}
	task.out.recordMutation(task.id, string(spec.Verb), int64(spec.Quantity), true, spec.Object)
	return nil
}

// validate rejects a content-free Effect: one with no known verb, no
// object, no positive quantity, or no callback to perform it.
func (s EffectSpec) validate(fn func(context.Context) error) error {
	switch {
	case !s.Verb.valid():
		return fmt.Errorf("%w: got %q", ErrEffectVerbInvalid, s.Verb)
	case strings.TrimSpace(s.Object) == "":
		return ErrEffectObjectMissing
	case s.Quantity <= 0:
		return fmt.Errorf("%w: %s %q has Quantity %d", ErrEffectQuantityNotPositive, s.Verb, s.Object, s.Quantity)
	case fn == nil:
		return ErrEffectCallbackMissing
	}
	return nil
}
