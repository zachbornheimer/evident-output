package engine

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/resource"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Resource names one unit of shared state an operation coordinates on
// (ZYS-840). It is sealed: only FSResource and LogicalResource make one.
type Resource = resource.Resource

// FSResource names the filesystem path path. A relative path resolves
// against the Run's workspace directory, the same way FileSpec.Path does,
// and symlinked ancestors resolve to their targets so an alias cannot
// bypass coordination. A claim on a directory overlaps every claim on a
// path beneath it. Construction performs no I/O.
func FSResource(path string) Resource { return resource.FS(path) }

// LogicalResource names non-filesystem shared state (a package manager, a
// remote, a database) where no truthful filesystem path exists. Two
// logical resources overlap only when their names match exactly after
// surrounding whitespace is trimmed; names are not hierarchical.
func LogicalResource(name string) Resource { return resource.Logical(name) }

// Resource access modes stay internal: callers never pick a mode, the
// operation (File, Basis, Effect) implies it.
const (
	resourceRead  = resource.Read
	resourceWrite = resource.Write
)

// processResources coordinates every Output in this process: two Outputs
// touching the same file must exclude each other just as two tasks in one
// Output do.
var processResources = resource.NewRegistry()

// holdResource holds r in mode for fn's duration and releases it however
// fn ends. It is the only way engine code acquires a resource; there is no
// lock/unlock pair to misuse. A ctx that already holds a resource fails
// with ErrNestedResourceAcquisition before any wait.
//
// An uncontended claim is invisible. When the claim has to wait and ctx
// belongs to a Task's Define callback, that Task shows "waiting for
// <resource>" as its live activity until the claim is granted.
func (o *Output) holdResource(ctx context.Context, r Resource, mode resource.Mode, fn func(context.Context) error) error {
	wait := o.resourceWaitFor(ctx, r)
	defer wait.clear()
	req := resource.Request{Resource: r, Workspace: o.workspace(), Mode: mode, OnContended: wait.show}
	return processResources.HoldResource(ctx, req, func(held context.Context) error {
		wait.clear()
		return runHoldingResource(held, fn)
	})
}

// runHoldingResource is the single frame every granted claim's work runs
// beneath, so Wait can tell from its own goroutine's stack that the caller
// holds a claim, and from processClaimOwners that the goroutine which
// started it does (see refuseWaitUnderClaim).
func runHoldingResource(held context.Context, fn func(context.Context) error) error {
	holdingFrames.note()
	heldClaims.Add(1)
	defer heldClaims.Add(-1)
	defer processClaimOwners.hold(currentGoroutine())()
	return fn(held)
}

// heldClaims counts the claims held across the process, so a Wait while
// none is held skips reading its own stack.
var heldClaims atomic.Int64

// holdingFrames marks runHoldingResource (see frameMarker).
var holdingFrames frameMarker

// refuseWaitUnderClaim returns ErrNestedResourceAcquisition, naming the
// awaited Task or container, when the calling goroutine, or the goroutine
// that started it, holds a resource claim. Waiting while holding a claim is nested acquisition in
// disguise: the awaited work may need the held resource, and neither side
// could then move. Like a second acquisition, it is refused every time,
// not only when it would actually conflict, so the outcome never depends
// on timing. Wait takes no context, so the claim is read from the stack.
func (o *Output) refuseWaitUnderClaim(ref string, stack *waiterStack) error {
	if !stack.holdsClaim() {
		return nil
	}
	return fmt.Errorf("%w: Wait on %q while holding a resource claim", ErrNestedResourceAcquisition, o.refName(ref))
}

// refName is the declared name of the Task or Group/Sequence ref names.
func (o *Output) refName(ref string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if st := o.taskByRef[ref]; st != nil {
		return st.name
	}
	if col := o.tasksByRef[ref]; col != nil {
		return col.name
	}
	return ref
}

// checkResourceFree fails with ErrNestedResourceAcquisition when ctx
// already holds a resource. Operations that may block before claiming
// their own resource (opening the cross-process manifest lock) call it
// first, so a held claim can never wait on anything else.
func checkResourceFree(ctx context.Context, r Resource, mode resource.Mode) error {
	return resource.CheckFree(ctx, fmt.Sprintf("%s %v", mode, r))
}

// validateResource resolves r without claiming it, so a dry run rejects
// an invalid Resource exactly like an applied run does.
func (o *Output) validateResource(r Resource) error {
	if _, err := resource.Resolve(r, o.workspace()); err != nil {
		return fmt.Errorf("evo: resource %v: %w", r, err)
	}
	return nil
}

// resourceWait is one claim's waiting activity: shown on its Task only if
// the claim is contended, and cleared the moment the claim is granted (or
// abandoned). show and clear run on the claiming goroutine.
type resourceWait struct {
	out    *Output
	taskID string
	text   string
	prior  string
	shown  bool
}

// resourceWaitFor prepares r's waiting activity for the Task ctx belongs
// to. Outside a Define callback there is no row to show it on, and show
// is a no-op.
func (o *Output) resourceWaitFor(ctx context.Context, r Resource) *resourceWait {
	wait := &resourceWait{out: o, text: txt.Text(resourceWaitingPrefix + resource.Label(r))}
	if task, err := taskScope(ctx); err == nil && task.out == o {
		wait.taskID = task.id
	}
	return wait
}

func (w *resourceWait) show(resource.Claim) {
	if w.taskID == "" {
		return
	}
	w.out.mu.Lock()
	defer w.out.mu.Unlock()
	st := w.out.taskByRef[w.taskID]
	if st == nil || core.IsTerminalTask(st.state) {
		return
	}
	w.prior = st.phase
	w.shown = true
	w.out.setLiveOnlyPhaseLocked(st, w.text)
}

// clear restores the Task's previous activity, unless something else
// replaced the waiting text in the meantime.
func (w *resourceWait) clear() {
	if !w.shown {
		return
	}
	w.shown = false
	w.out.mu.Lock()
	defer w.out.mu.Unlock()
	st := w.out.taskByRef[w.taskID]
	if st == nil || core.IsTerminalTask(st.state) || st.phase != w.text {
		return
	}
	w.out.setLiveOnlyPhaseLocked(st, w.prior)
}

// Resource misuse errors.
var (
	// ErrNestedResourceAcquisition is returned, without waiting, when code
	// already holding a resource (directly, or through any helper it passed
	// its context to) asks for a second one, or calls Wait on a Task, Group,
	// or Sequence. Holding at most one resource at a time, and never
	// waiting while holding one, is what makes deadlock impossible, so this
	// is misuse even when the second resource is free.
	//
	// Wait takes no context, so it reads the claim from its own goroutine's
	// stack: a goroutine the claim holder starts and then Waits from is not
	// caught, and can deadlock. Pass the held context instead, and never
	// Wait from a goroutine spawned while holding a resource.
	ErrNestedResourceAcquisition = resource.ErrNested
	// ErrInvalidResource is returned when a Resource names nothing: an
	// empty path or logical name.
	ErrInvalidResource = resource.ErrInvalid
)

// resourceWaitingPrefix opens the live activity a Task shows while one of
// its claims waits on a conflicting holder.
const resourceWaitingPrefix = "waiting for "
