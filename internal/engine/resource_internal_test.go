package engine

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

// renderDefine runs one Task whose Define body is body and returns the
// full plain rendering, so two bodies can be compared byte for byte.
func renderDefine(t *testing.T, body func(ctx context.Context) error) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	out := newOutput("resources", to(&buf), withNoColor(), plain())
	task := out.Task("sync config")
	task.Define(body)
	waitErr := task.Wait()
	_ = out.Finish()
	return buf.String(), waitErr
}

// A short uncontended acquisition is an implementation detail: the
// rendering must be byte-identical to the same task with no resource at
// all.
func TestHoldResource_UncontendedProducesNoOutput(t *testing.T) {
	dir := t.TempDir()
	bare, err := renderDefine(t, func(context.Context) error { return nil })
	if err != nil {
		t.Fatalf("bare task: %v", err)
	}
	if bare == "" {
		t.Fatal("bare task rendered nothing; the comparison would be vacuous")
	}
	var out *Output
	held, err := renderDefine(t, func(ctx context.Context) error {
		scoped, scopeErr := taskScope(ctx)
		if scopeErr != nil {
			return scopeErr
		}
		out = scoped.out
		return out.holdResource(ctx, FSResource(dir), resourceWrite, func(context.Context) error { return nil })
	})
	if err != nil {
		t.Fatalf("holding task: %v", err)
	}
	if out == nil {
		t.Fatal("holdResource never ran")
	}
	if held != bare {
		t.Fatalf("uncontended acquisition changed the rendering:\n--- bare ---\n%s\n--- held ---\n%s", bare, held)
	}
}

// writeAudit is the kind of helper a callback calls without knowing it
// acquires a resource of its own.
func writeAudit(ctx context.Context, out *Output) error {
	return out.holdResource(ctx, LogicalResource("audit-log"), resourceWrite, func(context.Context) error { return nil })
}

func TestHoldResource_NestedThroughHelperIsMisuse(t *testing.T) {
	dir := t.TempDir()
	_, err := renderDefine(t, func(ctx context.Context) error {
		scoped, scopeErr := taskScope(ctx)
		if scopeErr != nil {
			return scopeErr
		}
		return scoped.out.holdResource(ctx, FSResource(dir), resourceRead, func(held context.Context) error {
			return writeAudit(held, scoped.out)
		})
	})
	if !errors.Is(err, ErrNestedResourceAcquisition) {
		t.Fatalf("task error = %v, want ErrNestedResourceAcquisition", err)
	}
}

func TestHoldResource_InvalidResourceIsReported(t *testing.T) {
	_, err := renderDefine(t, func(ctx context.Context) error {
		scoped, scopeErr := taskScope(ctx)
		if scopeErr != nil {
			return scopeErr
		}
		return scoped.out.holdResource(ctx, LogicalResource(""), resourceRead, func(context.Context) error { return nil })
	})
	if !errors.Is(err, ErrInvalidResource) {
		t.Fatalf("task error = %v, want ErrInvalidResource", err)
	}
}
