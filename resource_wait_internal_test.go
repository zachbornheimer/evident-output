package evo

import (
	"context"
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
func runWithin(t *testing.T, out *Run, fn RunFunc) Result {
	t.Helper()
	done := make(chan Result, 1)
	go func() { done <- out.run(context.Background(), fn) }()
	select {
	case res := <-done:
		return res
	case <-time.After(waitUnderClaimDeadline):
		t.Fatalf("run still blocked after %s: Wait under a held resource claim deadlocked", waitUnderClaimDeadline)
		return Result{}
	}
}

// TestTaskHandle_Wait_WithoutClaimStillSucceeds guards against the fix being
// too broad: the same shape with no Resource on the Effect completes.
func TestTaskHandle_Wait_WithoutClaimStillSucceeds(t *testing.T) {
	dir := t.TempDir()
	out := initRun(Config{plain: true, color: colorNever, MaxConcurrency: 4, stdout: io.Discard, stderr: io.Discard})
	res := runWithin(t, out, func(ctx context.Context, r *Run) error {
		b := out.Task("write file")
		b.Define(func(ctx context.Context) error {
			return File(ctx, FileSpec{Path: filepath.Join(dir, "x"), Contents: []byte("hi")})
		})
		a := out.Task("effect")
		a.Define(func(ctx context.Context) error {
			spec := EffectSpec{Verb: Update, Object: "repo", Quantity: 1}
			return Effect(ctx, spec, func(context.Context) error { return b.wait() })
		})
		return a.wait()
	})
	if res.ExitCode() != exitOK {
		t.Fatalf("exit %d, want ExitOK", res.ExitCode())
	}
}
