package evo

import (
	"bytes"
	"context"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
)

// renderDefine runs one Task whose Define body is body and returns the
// full plain rendering, so two bodies can be compared byte for byte.
func renderDefine(t *testing.T, body func(ctx context.Context) error) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	out := newOutput("resources", to(&buf), withNoColor(), plain())
	task := out.Task("sync config")
	task.Define(body)
	waitErr := task.wait()
	_ = out.finish()
	return buf.String(), waitErr
}

// holdIn holds res in mode for fn, through the Run whose Define callback
// ctx belongs to.
func holdIn(ctx context.Context, res change.Resource, mode change.ClaimMode, fn func(context.Context) error) error {
	scope, err := runOf(ctx)
	if err != nil {
		return err
	}
	return scope.Owner().Hold(ctx, res, mode, fn)
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
	held := false
	rendered, err := renderDefine(t, func(ctx context.Context) error {
		return holdIn(ctx, change.FSResource(dir), change.ClaimWrite, func(context.Context) error {
			held = true
			return nil
		})
	})
	if err != nil {
		t.Fatalf("holding task: %v", err)
	}
	if !held {
		t.Fatal("the claimed work never ran")
	}
	if rendered != bare {
		t.Fatalf("uncontended acquisition changed the rendering:\n--- bare ---\n%s\n--- held ---\n%s", bare, rendered)
	}
}
