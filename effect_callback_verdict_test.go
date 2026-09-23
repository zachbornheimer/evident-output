package evo_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestEffect_CallbackThatSkippedItsOwnTaskRecordsNoEffect is the red-first
// proof for the canary's `create-skipped` probe: an Effect callback that
// resolved its own task as Skipped and returned nil still put a `[changed]`
// row in the ledger, so the ledger counted the package the installer had
// just rejected. A nil return after the callback said "skipped" means "I
// handled it", not "I did it"; the effect ledger must not count what the row
// itself denies.
func TestEffect_CallbackThatSkippedItsOwnTaskRecordsNoEffect(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Title: "setup", Stdout: &buf, Plain: true, Color: evo.ColorNever})

	install := out.Group("install")
	broken := install.Task("broken")
	broken.Define(func(ctx context.Context) error {
		return evo.Effect(ctx, createModule, func(context.Context) error {
			broken.Skipped(evo.Reason("install failed"))
			return nil
		})
	})
	ok := install.Task("ok")
	ok.Define(func(ctx context.Context) error {
		return evo.Effect(ctx, createModule, func(context.Context) error { return nil })
	})
	if err := out.Finish(); err != nil {
		t.Log(err)
	}

	got := buf.String()
	if strings.Contains(got, "[changed] broken") {
		t.Fatalf("a skipped callback must record no effect:\n%s", got)
	}
	if !strings.Contains(got, "created 1 module") {
		t.Fatalf("the callback that did create must still be in the ledger:\n%s", got)
	}
}

// TestEffect_CallbackThatFailedItsOwnTaskRecordsNoEffect is the same rule
// on the other terminal verdict: an Effect callback that called Fail on its
// own task and then returned nil claims no mutation — and is not misuse.
func TestEffect_CallbackThatFailedItsOwnTaskRecordsNoEffect(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Title: "setup", Stdout: &buf, Plain: true, Color: evo.ColorNever})

	broken := out.Task("broken")
	broken.Define(func(ctx context.Context) error {
		return evo.Effect(ctx, createModule, func(context.Context) error {
			broken.Fail("uv rejected the package", evo.Detail("resolution impossible"))
			return nil
		})
	})
	if err := out.Finish(); err != nil {
		t.Log(err)
	}

	if got := buf.String(); strings.Contains(got, "[changed]") {
		t.Fatalf("a failed callback must record no effect:\n%s", got)
	}
	if err := out.Err(); err != nil {
		t.Fatalf("a callback that disowned its own Effect is not misuse, got %v", err)
	}
}

// TestEffect_CallbackThatDoneItsOwnTaskKeepsTheEffect guards the fix's
// blast radius: `Done(summary)` inside an Effect callback is the ratified
// proposal shape the dialect teaches, and it still records the effect.
func TestEffect_CallbackThatDoneItsOwnTaskKeepsTheEffect(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Title: "setup", Stdout: &buf, Plain: true, Color: evo.ColorNever})

	pkg := out.Task("numpy")
	pkg.Define(func(ctx context.Context) error {
		return evo.Effect(ctx, createModule, func(context.Context) error {
			pkg.Done("installed from cache")
			return nil
		})
	})
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	if got := buf.String(); !strings.Contains(got, "created 1 module") {
		t.Fatalf("a ratified Done proposal must keep its effect:\n%s", got)
	}
}

// createModule is the one-module Effect these verdict tests share.
var createModule = evo.EffectSpec{Verb: evo.EffectCreate, Object: "module", Quantity: 1}
