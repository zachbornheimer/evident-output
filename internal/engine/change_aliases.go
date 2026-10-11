package engine

import (
	"context"
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/change"
)

// The changes a Task's callback performs live in internal/change. Engine
// aliases them so the root package keeps compiling through its own aliases
// until it speaks internal/change directly.

type (
	// EffectVerb is the closed set of imperative verbs an opaque Effect may
	// declare. There is deliberately no write verb: file state goes through
	// File (and Patch), never an opaque callback.
	EffectVerb = change.EffectVerb
	// EffectSpec describes one aggregate opaque mutation Evo cannot model as
	// desired state. Constructing an EffectSpec performs no I/O.
	EffectSpec = change.EffectSpec
	// Resource names one unit of shared state an operation coordinates on
	// (ZYS-840). It is sealed: only FSResource and LogicalResource make one.
	Resource = change.Resource
)

// The complete EffectVerb enum.
const (
	EffectAdd       = change.EffectAdd
	EffectCreate    = change.EffectCreate
	EffectDelete    = change.EffectDelete
	EffectInstall   = change.EffectInstall
	EffectPush      = change.EffectPush
	EffectRemove    = change.EffectRemove
	EffectUninstall = change.EffectUninstall
	EffectUpdate    = change.EffectUpdate
)

// Effect performs one opaque mutation inside a Task's Define callback; see
// change.Effect.
func Effect(ctx context.Context, spec EffectSpec, fn func(context.Context) error) error {
	return change.Effect(ctx, spec, fn)
}

// PartialEffect reports, from an Effect callback, that committed of the
// requested EffectSpec.Quantity really happened before err stopped the rest;
// see change.PartialEffect.
func PartialEffect(committed int, err error) error { return change.PartialEffect(committed, err) }

// FSResource names the filesystem path path. A relative path resolves
// against the Run's workspace directory, the same way FileSpec.Path does,
// and symlinked ancestors resolve to their targets so an alias cannot bypass
// coordination. A claim on a directory overlaps every claim on a path
// beneath it. Construction performs no I/O.
func FSResource(path string) Resource { return change.FSResource(path) }

// LogicalResource names non-filesystem shared state (a package manager, a
// remote, a database) where no truthful filesystem path exists. Two logical
// resources overlap only when their names match exactly after surrounding
// whitespace is trimmed; names are not hierarchical.
func LogicalResource(name string) Resource { return change.LogicalResource(name) }

// Effect, PartialEffect and resource sentinels.
var (
	ErrEffectVerbInvalid         = change.ErrEffectVerbInvalid
	ErrEffectObjectMissing       = change.ErrEffectObjectMissing
	ErrEffectQuantityNotPositive = change.ErrEffectQuantityNotPositive
	ErrEffectCallbackMissing     = change.ErrEffectCallbackMissing
	ErrInvalidPartialEffect      = change.ErrInvalidPartialEffect
	// ErrNestedResourceAcquisition is returned, without waiting, when code
	// already holding a resource (directly, or through any helper it passed
	// its context to) asks for a second one, or calls Wait on a Task, Group,
	// or Sequence. Holding at most one resource at a time, and never waiting
	// while holding one, is what makes deadlock impossible, so this is
	// misuse even when the second resource is free.
	//
	// Wait takes no context, so it reads the claim from its own goroutine's
	// stack: a goroutine the claim holder starts and then Waits from is not
	// caught, and can deadlock. Pass the held context instead, and never
	// Wait from a goroutine spawned while holding a resource.
	ErrNestedResourceAcquisition = change.ErrNestedResourceAcquisition
	// ErrInvalidResource is returned when a Resource names nothing: an empty
	// path or logical name.
	ErrInvalidResource = change.ErrInvalidResource
)

// Resource access modes stay internal: callers never pick a mode, the
// operation (File, Basis, Effect) implies it.
const (
	resourceRead  = change.ClaimRead
	resourceWrite = change.ClaimWrite
)

// checkResourceFree fails with ErrNestedResourceAcquisition when ctx already
// holds a resource. Operations that may block before claiming their own
// resource (opening the cross-process manifest lock) call it first, so a held
// claim can never wait on anything else.
func checkResourceFree(ctx context.Context, r Resource, mode change.ClaimMode) error {
	return change.CheckClaimFree(ctx, fmt.Sprintf("%s %v", mode, r))
}

// holdResource holds r in mode for fn's duration; see change.Run.Hold.
func (o *Output) holdResource(ctx context.Context, r Resource, mode change.ClaimMode, fn func(context.Context) error) error {
	return o.changes.Hold(ctx, r, mode, fn)
}
