// This file owns Effect: one opaque mutation inside a Task's Define callback,
// recorded in the ledger as planned or changed.

package change

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

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
	// Path is an optional filesystem path the Effect claims for writing
	// while fn runs: Effects whose Paths are equal or one contains the
	// other wait for each other. A relative Path resolves against the Run's
	// workspace (see FSResource).
	Path string
	// Resource is an optional logical resource name the Effect claims for
	// writing while fn runs: Effects whose names match after surrounding
	// whitespace is trimmed wait for each other (see LogicalResource). An
	// Effect claims at most one of Path and Resource, and a waiting Task
	// shows "waiting for <claim>".
	Resource string
}

// Effect usage errors: a content-free Effect describes no mutation the
// ledger could truthfully record, so Effect refuses it without invoking fn.
var (
	ErrEffectVerbInvalid         = errors.New("evo: EffectSpec.Verb must be one of the EffectVerb constants")
	ErrEffectObjectMissing       = errors.New("evo: EffectSpec.Object is required")
	ErrEffectQuantityNotPositive = errors.New("evo: EffectSpec.Quantity must be > 0")
	ErrEffectCallbackMissing     = errors.New("evo: Effect requires a non-nil callback")
	ErrEffectClaimsTwoResources  = fmt.Errorf("%w: EffectSpec sets both Path and Resource; an Effect claims one", graph.ErrInvalidConfig)
)

// Effect performs one opaque mutation inside a Task's Define callback (ctx
// must come from one; see graph.ScopeOf). A dry run records one planned
// Effect and never invokes fn. An apply run invokes fn with ctx — the
// scheduler-owned context — and records the changed Effect only when fn
// returns nil; fn's error is returned unchanged so Define can return it,
// except that a PartialEffect records its committed subset first (see
// recordPartialEffect) and an invalid one wraps ErrInvalidPartialEffect. A
// callback that resolved its own task as anything but Done (Skipped, Fail,
// Block) disowned the work, so nothing reaches the ledger.
func Effect(ctx context.Context, spec EffectSpec, fn func(context.Context) error) error {
	scope, beginErr := graph.BeginOperation[*Run](ctx, fmt.Sprintf("Effect %s %q", spec.Verb, spec.Object))
	if beginErr != nil {
		return beginErr
	}
	return scope.Owner().effect(ctx, scope.Task(), spec, fn)
}

func (r *Run) effect(ctx context.Context, id record.TaskID, spec EffectSpec, fn func(context.Context) error) error {
	if invalid := spec.validate(fn); invalid != nil {
		return invalid
	}
	// Resolve the ledger once, before fn runs: an interrupt that cancels the
	// row while fn runs describes work that really happened, and the reader
	// is still owed "! already mutated: ...".
	ledger, ledgerErr := r.host.Ledger(id)
	if ledgerErr != nil {
		return ledgerErr
	}
	if ledger.Applies() {
		disowned, callbackErr := r.runEffectCallback(ctx, id, func(ctx context.Context) error {
			return r.performEffect(ctx, spec.claim(), fn)
		})
		if callbackErr != nil {
			return recordPartialEffect(ledger, spec, callbackErr)
		}
		if disowned {
			return nil
		}
	} else if claim := spec.claim(); claim != nil {
		if resourceErr := r.validateResource(claim); resourceErr != nil {
			return resourceErr
		}
	}
	ledger.Record(spec.entry(spec.Quantity))
	return nil
}

// recordPartialEffect handles a failed Effect callback: when its error is a
// valid PartialEffect with a positive committed count, that subset is
// recorded as changed (the original Verb/Object, Quantity=committed) before
// the error is returned, so the Task fails over a truthful ledger. The
// callback's own verdict on its row does not erase work it says committed.
func recordPartialEffect(ledger Ledger, spec EffectSpec, callbackErr error) error {
	committed, effectErr := spec.committedOf(callbackErr)
	if committed > 0 {
		ledger.Record(spec.entry(committed))
	}
	return effectErr
}

// performEffect invokes fn, holding res for writing when the Effect claims
// one. fn receives the holding context, so tracked work inside it that would
// need a second resource fails as nested acquisition.
func (r *Run) performEffect(ctx context.Context, res Resource, fn func(context.Context) error) error {
	if res == nil {
		return fn(ctx)
	}
	return r.Hold(ctx, res, ClaimWrite, fn)
}

// validate rejects a content-free Effect (one with no known verb, no object,
// no positive quantity, or no callback to perform it) and one that claims
// both a Path and a Resource.
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
	case s.Path != "" && s.Resource != "":
		return fmt.Errorf("%w: Path %q, Resource %q", ErrEffectClaimsTwoResources, s.Path, s.Resource)
	}
	return nil
}

// claim is the one Resource s holds while its callback runs, or nil when it
// names none. A whitespace-only name still claims, so resolving the claim
// rejects it instead of the Effect silently running unclaimed.
func (s EffectSpec) claim() Resource {
	switch {
	case s.Path != "":
		return FSResource(s.Path)
	case s.Resource != "":
		return LogicalResource(s.Resource)
	default:
		return nil
	}
}

// entry is s's ledger row for quantity objects: s.Quantity for a committed
// Effect, the committed subset for a PartialEffect.
func (s EffectSpec) entry(quantity int) record.LedgerEntry {
	return record.CountedEntry(string(s.Verb), s.Object, quantity)
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
func (r *Run) runEffectCallback(ctx context.Context, id record.TaskID, fn func(context.Context) error) (disowned bool, err error) {
	deniedAtEntry := r.effects.enter(id)
	defer func() { disowned = r.effects.exit(id, deniedAtEntry) }()
	return false, fn(ctx)
}
