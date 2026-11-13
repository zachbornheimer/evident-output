package engine

import "context"

// taskScopeContextKey is the unexported context.Value key a Define
// callback's context carries — never a plain string, so nothing outside
// this package can collide with or forge it.
type taskScopeContextKey struct{}

// taskScopeHandle is one Define execution's task scope: which Task owns it,
// and whether that execution has already returned. File/Exec (next
// increment) read this through taskScope to refuse work outside a callback
// or after it closed (§7.1).
type taskScopeHandle struct {
	out    *Output
	taskID string
	// closed is true once the Define callback that opened this scope has
	// returned. A context captured during the callback and reused after is
	// exactly the misuse ErrTaskClosed reports.
	closed bool
}

// withTaskScope derives a context carrying scope, for the Define callback
// (and, in a later increment, evo.File/evo.Exec) to read back via taskScope.
func withTaskScope(ctx context.Context, scope *taskScopeHandle) context.Context {
	return context.WithValue(ctx, taskScopeContextKey{}, scope)
}

// closeTaskScopeLocked marks scope closed. Callers must already hold
// scope.out.mu — the same lock every other taskState field is guarded by,
// since a concurrent taskScope(ctx) call reads scope.closed under it too.
func closeTaskScopeLocked(scope *taskScopeHandle) {
	if scope != nil {
		scope.closed = true
	}
}

// taskScope returns the Task ctx's Define callback is running under.
//
//   - ErrNoTaskContext: ctx carries no task scope at all — it did not come
//     from a Define callback (a bare context.Background(), or one from
//     outside any callback).
//   - ErrTaskClosed: ctx carries a scope, but its Define callback has
//     already returned — the context was captured during the callback and
//     used again afterward.
//   - otherwise: the Running Task the callback is (or was) executing.
func taskScope(ctx context.Context) (*TaskHandle, error) {
	if ctx == nil {
		return nil, ErrNoTaskContext
	}
	scope, ok := ctx.Value(taskScopeContextKey{}).(*taskScopeHandle)
	if !ok || scope == nil || scope.out == nil {
		return nil, ErrNoTaskContext
	}
	scope.out.mu.Lock()
	closed := scope.closed
	scope.out.mu.Unlock()
	if closed {
		return nil, ErrTaskClosed
	}
	return &TaskHandle{out: scope.out, id: scope.taskID}, nil
}
