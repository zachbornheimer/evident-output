package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// EffectVerb is the closed set of imperative verbs an opaque Effect may
// declare. There is deliberately no write verb: file state goes through
// File (and Patch), never an opaque callback.
type EffectVerb string

// The complete EffectVerb enum.
const (
	EffectAdd       EffectVerb = core.EffectVerbAdd
	EffectCreate    EffectVerb = core.EffectVerbCreate
	EffectDelete    EffectVerb = core.EffectVerbDelete
	EffectInstall   EffectVerb = core.EffectVerbInstall
	EffectPush      EffectVerb = core.EffectVerbPush
	EffectRemove    EffectVerb = core.EffectVerbRemove
	EffectUninstall EffectVerb = core.EffectVerbUninstall
	EffectUpdate    EffectVerb = core.EffectVerbUpdate
)

// valid reports whether v is one of the declared EffectVerb constants.
func (v EffectVerb) valid() bool {
	_, ok := core.EffectVerbConstant(string(v))
	return ok
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
	// Resource is an optional single resource the Effect claims for
	// writing while fn runs; nil means none. Overlapping claims (the same
	// logical name, or filesystem paths where one contains the other) wait
	// for each other, and a waiting Task shows "waiting for <resource>".
	Resource Resource
}

// Effect usage errors: a content-free Effect describes no mutation the
// ledger could truthfully record, so Effect refuses it without invoking fn.
var (
	ErrEffectVerbInvalid         = errors.New("evo: EffectSpec.Verb must be one of the EffectVerb constants")
	ErrEffectObjectMissing       = errors.New("evo: EffectSpec.Object is required")
	ErrEffectQuantityNotPositive = errors.New("evo: EffectSpec.Quantity must be > 0")
	ErrEffectCallbackMissing     = errors.New("evo: Effect requires a non-nil callback")
	ErrEffectMeasuredNegative    = errors.New("evo: MeasuredEffect callback returned a negative quantity")
)

// Effect performs one opaque mutation inside a Task's Define callback
// (ctx must come from one; see taskScope). A dry run records one planned
// Effect and never invokes fn. An apply run invokes fn with ctx — the
// scheduler-owned context — and records the changed Effect only when fn
// returns nil; fn's error is returned unchanged so Define can return it,
// except that a PartialEffect records its committed subset first (see
// recordPartialEffect) and an invalid one wraps ErrInvalidPartialEffect. A
// callback that resolved its own task as anything but Done (Skipped, Fail,
// Block) disowned the work, so nothing reaches the ledger (see
// deniesItsOwnEffect).
func Effect(ctx context.Context, spec EffectSpec, fn func(context.Context) error) error {
	if fn == nil {
		return effectMeasuring(ctx, spec, nil)
	}
	return effectMeasuring(ctx, spec, func(ctx context.Context) (int, error) {
		return spec.Quantity, fn(ctx)
	})
}

// MeasuredEffect is Effect for work whose size is only known once it ran:
// fn returns the quantity it really affected, and the changed ledger row and
// the closing summary record that instead of spec.Quantity, which stays the
// plan a dry run shows. A returned 0 records no Effect, like a callback that
// found nothing to mutate; a negative quantity fails with
// ErrEffectMeasuredNegative and records nothing. fn's error and a
// PartialEffect behave exactly as in Effect (the PartialEffect's committed
// count, not the returned quantity, is recorded on failure).
func MeasuredEffect(ctx context.Context, spec EffectSpec, fn func(context.Context) (int, error)) error {
	return effectMeasuring(ctx, spec, fn)
}

// effectMeasuring is the one Effect lifecycle: run reports the quantity the
// ledger records when it succeeds, or nil for a missing callback.
func effectMeasuring(ctx context.Context, spec EffectSpec, run func(context.Context) (int, error)) error {
	task, err := beginOperation(ctx, fmt.Sprintf("Effect %s %q", spec.Verb, spec.Object))
	if err != nil {
		return err
	}
	if err := spec.validate(run != nil); err != nil {
		return err
	}
	// Resolve the ledger target once, before fn runs: an interrupt that
	// cancels the row while fn runs describes work that really happened,
	// and the reader is still owed "! already mutated: ...".
	target, err := task.out.resolveLedgerTarget(task.id)
	if err != nil {
		return err
	}
	quantity := spec.Quantity
	if target.tense == tenseChanged {
		disowned, err := task.out.runEffectCallback(ctx, task.id, func(ctx context.Context) error {
			return task.out.performEffect(ctx, spec.Resource, func(ctx context.Context) error {
				measured, err := run(ctx)
				if err == nil {
					quantity = measured
				}
				return err
			})
		})
		if err != nil {
			return task.out.recordPartialEffect(task.id, target, spec, err)
		}
		if disowned {
			return nil
		}
	} else if spec.Resource != nil {
		if err := task.out.validateResource(spec.Resource); err != nil {
			return err
		}
	}
	if quantity < 0 {
		return fmt.Errorf("%w: %s %q measured %d", ErrEffectMeasuredNegative, spec.Verb, spec.Object, quantity)
	}
	if quantity == 0 {
		return nil
	}
	task.out.recordResolvedEntry(task.id, target, spec.entry(quantity))
	return nil
}

// recordPartialEffect handles a failed Effect callback: when its error is a
// valid PartialEffect with a positive committed count, that subset is
// recorded as changed (the original Verb/Object, Quantity=committed) before
// the error is returned, so the Task fails over a truthful ledger. The
// callback's own verdict on its row does not erase work it says committed.
func (o *Output) recordPartialEffect(taskID string, target ledgerTarget, spec EffectSpec, err error) error {
	committed, err := spec.committedOf(err)
	if committed > 0 {
		o.recordResolvedEntry(taskID, target, spec.entry(committed))
	}
	return err
}

// performEffect invokes fn, holding r for writing when the Effect claims
// one. fn receives the holding context, so tracked work inside it that
// would need a second resource fails as nested acquisition.
func (o *Output) performEffect(ctx context.Context, r Resource, fn func(context.Context) error) error {
	if r == nil {
		return fn(ctx)
	}
	return o.holdResource(ctx, r, resourceWrite, fn)
}

// validate rejects a content-free Effect: one with no known verb, no
// object, no positive quantity, or no callback to perform it.
func (s EffectSpec) validate(hasCallback bool) error {
	switch {
	case !s.Verb.valid():
		return fmt.Errorf("%w: got %q", ErrEffectVerbInvalid, s.Verb)
	case strings.TrimSpace(s.Object) == "":
		return ErrEffectObjectMissing
	case s.Quantity <= 0:
		return fmt.Errorf("%w: %s %q has Quantity %d", ErrEffectQuantityNotPositive, s.Verb, s.Object, s.Quantity)
	case !hasCallback:
		return ErrEffectCallbackMissing
	}
	return nil
}

// runEffectCallback invokes an Effect's fn with the task marked as having an
// Effect in flight, and reports whether the work was disowned: the task
// resolved itself as anything but Done from inside a callback while fn ran.
// Each invocation compares the task's denial count at its own entry and
// exit. The count is per task, and a resolving call cannot say which
// callback it came from, so when one Define runs Effects concurrently, a
// denial from any of them disowns every Effect in flight at that moment:
// their rows record nothing, matching the task's own "I handled it"
// verdict.
func (o *Output) runEffectCallback(ctx context.Context, taskID string, fn func(context.Context) error) (disowned bool, err error) {
	o.mu.Lock()
	st := o.taskByRef[taskID]
	var deniedAtEntry int
	if st != nil {
		st.effectsInFlight++
		deniedAtEntry = st.effectDenials
	}
	o.mu.Unlock()
	defer func() {
		o.mu.Lock()
		defer o.mu.Unlock()
		if st == nil {
			return
		}
		st.effectsInFlight--
		disowned = st.effectDenials != deniedAtEntry
	}()
	return false, fn(ctx)
}
