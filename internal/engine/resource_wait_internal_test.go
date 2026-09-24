package engine

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"
)

// waitUnderClaimDeadline bounds how long the held-claim Wait tests let a run
// take before calling it hung.
const waitUnderClaimDeadline = 5 * time.Second

// runWithin runs fn in a Run on out and fails the test if it does not return
// within waitUnderClaimDeadline: the failure mode these tests pin is a hang.
func runWithin(t *testing.T, out *Output, fn RunFunc) Result {
	t.Helper()
	done := make(chan Result, 1)
	go func() { done <- out.Run(context.Background(), fn) }()
	select {
	case res := <-done:
		return res
	case <-time.After(waitUnderClaimDeadline):
		t.Fatalf("run still blocked after %s: Wait under a held resource claim deadlocked", waitUnderClaimDeadline)
		return Result{}
	}
}

// TestTaskHandle_Wait_UnderHeldClaimIsNestedAcquisition pins the one-resource
// rule through Wait: an Effect holding FSResource(dir) that waits on a Task
// needing dir/x would deadlock (the waiter holds what the awaited work
// needs), so the Wait fails as nested acquisition instead of blocking.
func TestTaskHandle_Wait_UnderHeldClaimIsNestedAcquisition(t *testing.T) {
	dir := t.TempDir()
	out := Init(Config{Isolated: true, Plain: true, Color: ColorNever, MaxConcurrency: 4, Stdout: io.Discard, Stderr: io.Discard})
	var waitErr error
	runWithin(t, out, func(ctx context.Context) error {
		b := out.Task("write file")
		b.Define(func(ctx context.Context) error {
			return File(ctx, FileSpec{Path: filepath.Join(dir, "x"), Contents: []byte("hi")})
		})
		a := out.Task("effect")
		a.Define(func(ctx context.Context) error {
			spec := EffectSpec{Verb: EffectUpdate, Object: "repo", Quantity: 1, Resource: FSResource(dir)}
			return Effect(ctx, spec, func(context.Context) error {
				waitErr = b.Wait()
				return waitErr
			})
		})
		_ = a.Wait()
		return nil
	})
	if !errors.Is(waitErr, ErrNestedResourceAcquisition) {
		t.Fatalf("Wait under a held claim = %v, want ErrNestedResourceAcquisition", waitErr)
	}
}

// TestGroupHandle_Wait_UnderHeldClaimIsNestedAcquisition is the container
// counterpart: Group.Wait under a held claim fails once, as nested
// acquisition, rather than parking on children that need the claim.
func TestGroupHandle_Wait_UnderHeldClaimIsNestedAcquisition(t *testing.T) {
	dir := t.TempDir()
	out := Init(Config{Isolated: true, Plain: true, Color: ColorNever, MaxConcurrency: 4, Stdout: io.Discard, Stderr: io.Discard})
	var waitErr error
	runWithin(t, out, func(ctx context.Context) error {
		files := out.Group("files")
		files.Task("x").Define(func(ctx context.Context) error {
			return File(ctx, FileSpec{Path: filepath.Join(dir, "x"), Contents: []byte("hi")})
		})
		a := out.Task("effect")
		a.Define(func(ctx context.Context) error {
			spec := EffectSpec{Verb: EffectUpdate, Object: "repo", Quantity: 1, Resource: FSResource(dir)}
			return Effect(ctx, spec, func(context.Context) error {
				waitErr = files.Wait()
				return waitErr
			})
		})
		_ = a.Wait()
		return nil
	})
	if !errors.Is(waitErr, ErrNestedResourceAcquisition) {
		t.Fatalf("Group.Wait under a held claim = %v, want ErrNestedResourceAcquisition", waitErr)
	}
}

// TestTaskHandle_Wait_WithoutClaimStillSucceeds guards against the fix being
// too broad: the same shape with no Resource on the Effect completes.
func TestTaskHandle_Wait_WithoutClaimStillSucceeds(t *testing.T) {
	dir := t.TempDir()
	out := Init(Config{Isolated: true, Plain: true, Color: ColorNever, MaxConcurrency: 4, Stdout: io.Discard, Stderr: io.Discard})
	res := runWithin(t, out, func(ctx context.Context) error {
		b := out.Task("write file")
		b.Define(func(ctx context.Context) error {
			return File(ctx, FileSpec{Path: filepath.Join(dir, "x"), Contents: []byte("hi")})
		})
		a := out.Task("effect")
		a.Define(func(ctx context.Context) error {
			spec := EffectSpec{Verb: EffectUpdate, Object: "repo", Quantity: 1}
			return Effect(ctx, spec, func(context.Context) error { return b.Wait() })
		})
		return a.Wait()
	})
	if res.ExitCode() != ExitOK {
		t.Fatalf("exit %d, want ExitOK", res.ExitCode())
	}
}
