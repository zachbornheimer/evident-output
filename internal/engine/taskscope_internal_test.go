package engine

import (
	"context"
	"errors"
	"testing"
)

// TestTaskScope_NoScopeReturnsErrNoTaskContext proves §7.1: a context that
// never came from a Define callback (context.Background(), or any ctx not
// derived through withTaskScope) reports ErrNoTaskContext — the contract
// File/Exec (next increment) refuse work under.
func TestTaskScope_NoScopeReturnsErrNoTaskContext(t *testing.T) {
	if _, err := taskScope(context.Background()); !errors.Is(err, ErrNoTaskContext) {
		t.Fatalf("taskScope(context.Background()) = %v, want ErrNoTaskContext", err)
	}
}

// TestTaskScope_InsideCallbackReturnsTheRunningTask proves §7.1: inside a
// Define callback, taskScope(ctx) resolves to the Task actually running —
// same Output, same task ID.
func TestTaskScope_InsideCallbackReturnsTheRunningTask(t *testing.T) {
	out := Init(Config{Isolated: true})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("scoped")
	var gotID string
	var scopeErr error
	task.Define(func(ctx context.Context) error {
		scoped, err := taskScope(ctx)
		scopeErr = err
		if scoped != nil {
			gotID = scoped.id
		}
		return nil
	})
	_ = task.Wait()

	if scopeErr != nil {
		t.Fatalf("taskScope inside the callback returned %v, want nil", scopeErr)
	}
	if gotID != task.id {
		t.Fatalf("taskScope resolved id %q, want the running task's own id %q", gotID, task.id)
	}
}

// TestTaskScope_AfterCallbackReturnsErrTaskClosed proves §7.1: a context
// captured during a Define callback and reused after that callback
// returned reports ErrTaskClosed — the callback's own execution window is
// what "open" means, not the Task's lifetime.
func TestTaskScope_AfterCallbackReturnsErrTaskClosed(t *testing.T) {
	out := Init(Config{Isolated: true})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("closes")
	var captured context.Context
	task.Define(func(ctx context.Context) error {
		captured = ctx
		return nil
	})
	_ = task.Wait()

	if _, err := taskScope(captured); !errors.Is(err, ErrTaskClosed) {
		t.Fatalf("taskScope after the callback returned = %v, want ErrTaskClosed", err)
	}
}
