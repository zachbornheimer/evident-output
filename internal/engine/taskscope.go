package engine

import "context"

// taskScopeContextKey is the unexported context.Value key a Define
// callback's context carries — never a plain string, so nothing outside
// this package can collide with or forge it.
type taskScopeContextKey struct{}

// taskScopeHandle is one Define execution's task scope: which Task owns it,
// and whether that execution has already returned. File/Patch/Exec read
// this through taskScope to refuse work outside a callback or after it
// closed (§7.1).
type taskScopeHandle struct {
	out    *Output
	taskID string
	// closed is true once the Define callback that opened this scope has
	// returned. A context captured during the callback and reused after is
	// exactly the misuse ErrTaskClosed reports.
	closed bool
	// resourceHeld is true while this callback occupies one public Evo
	// resource (File, Patch, or Exec). A second occupancy on the same ctx
	// returns ErrNestedResource.
	resourceHeld bool
}

// withTaskScope derives a context carrying scope, for the Define callback
// (and File/Patch/Exec) to read back via taskScope.
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

// taskScopeFrom returns the Define-callback scope ctx carries.
//
//   - ErrNoTaskContext: ctx carries no task scope at all — it did not come
//     from a Define callback (a bare context.Background(), or one from
//     outside any callback).
//   - ErrTaskClosed: ctx carries a scope, but its Define callback has
//     already returned — the context was captured during the callback and
//     used again afterward.
func taskScopeFrom(ctx context.Context) (*taskScopeHandle, error) {
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
	return scope, nil
}

// taskScope returns the Task ctx's Define callback is running under.
func taskScope(ctx context.Context) (*TaskHandle, error) {
	scope, err := taskScopeFrom(ctx)
	if err != nil {
		return nil, err
	}
	return &TaskHandle{out: scope.out, id: scope.taskID}, nil
}

func (s *taskScopeHandle) beginPublicResource() error {
	s.out.mu.Lock()
	defer s.out.mu.Unlock()
	if s.closed {
		return ErrTaskClosed
	}
	if s.resourceHeld {
		return ErrNestedResource
	}
	s.resourceHeld = true
	return nil
}

func (s *taskScopeHandle) endPublicResource() {
	s.out.mu.Lock()
	defer s.out.mu.Unlock()
	s.resourceHeld = false
}

func beginPublicResource(ctx context.Context) (*taskScopeHandle, *TaskHandle, error) {
	scope, err := taskScopeFrom(ctx)
	if err != nil {
		return nil, nil, err
	}
	if err := scope.beginPublicResource(); err != nil {
		return nil, nil, err
	}
	return scope, &TaskHandle{out: scope.out, id: scope.taskID}, nil
}
