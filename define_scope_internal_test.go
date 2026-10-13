package evo

import (
	"context"
	"errors"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/change"
	"github.com/zachbornheimer/evident-output/internal/graph"
)

// runOf is the Run whose changes the Define callback ctx belongs to, or the
// reason ctx belongs to none.
func runOf(ctx context.Context) (*graph.Scope[*change.Run], error) {
	return graph.ScopeOf[*change.Run](ctx)
}

// TestDefineScope_InsideCallbackNamesTheRunningTaskAndItsRun proves §7.1:
// inside a Define callback the context's scope resolves to the Task actually
// running and to the Run's own changes.
func TestDefineScope_InsideCallbackNamesTheRunningTaskAndItsRun(t *testing.T) {
	out := Init(Config{Isolated: true})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("scoped")
	var gotID string
	var gotRun *change.Run
	var scopeErr error
	task.Define(func(ctx context.Context) error {
		scope, err := runOf(ctx)
		scopeErr = err
		if scope != nil {
			gotID, gotRun = string(scope.Task()), scope.Owner()
		}
		return nil
	})
	_ = task.Wait()

	if scopeErr != nil {
		t.Fatalf("the scope inside the callback returned %v, want nil", scopeErr)
	}
	if gotID != task.id || gotRun != out.changes {
		t.Fatalf("scope resolved (%q, %p), want the running task's own id %q and this Output's changes %p", gotID, gotRun, task.id, out.changes)
	}
}

// TestDefineScope_AfterCallbackReturnsErrTaskClosed proves §7.1: a context
// captured during a Define callback and reused after that callback returned
// reports ErrTaskClosed — the callback's own execution window is what "open"
// means, not the Task's lifetime.
func TestDefineScope_AfterCallbackReturnsErrTaskClosed(t *testing.T) {
	out := Init(Config{Isolated: true})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("closes")
	var captured context.Context
	task.Define(func(ctx context.Context) error {
		captured = ctx
		return nil
	})
	_ = task.Wait()

	if _, err := runOf(captured); !errors.Is(err, ErrTaskClosed) {
		t.Fatalf("the scope after the callback returned = %v, want ErrTaskClosed", err)
	}
}

// TestDefineScope_OutsideACallbackReturnsErrNoTaskContext proves §7.1: a
// context that never came from a Define callback reports ErrNoTaskContext.
func TestDefineScope_OutsideACallbackReturnsErrNoTaskContext(t *testing.T) {
	if _, err := runOf(context.Background()); !errors.Is(err, ErrNoTaskContext) {
		t.Fatalf("the scope of context.Background() = %v, want ErrNoTaskContext", err)
	}
}
