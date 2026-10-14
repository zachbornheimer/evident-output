package evo

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// TestEffect_CallbackThatFailedItsOwnTaskRecordsNoEffect is the same rule
// on the other terminal verdict: an Effect callback that called Fail on its
// own task and then returned nil claims no mutation — and is not misuse.
func TestEffect_CallbackThatFailedItsOwnTaskRecordsNoEffect(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := initRun(Config{title: "setup", stdout: &buf, plain: true, color: colorNever})

	broken := out.Task("broken")
	broken.Define(func(ctx context.Context) error {
		return Effect(ctx, createModule, func(context.Context) error {
			broken.fail("uv rejected the package", Detail("resolution impossible"))
			return nil
		})
	})
	if err := out.finish(); err != nil {
		t.Log(err)
	}

	if got := buf.String(); strings.Contains(got, "[changed]") {
		t.Fatalf("a failed callback must record no effect:\n%s", got)
	}
	if err := out.err(); err != nil {
		t.Fatalf("a callback that disowned its own Effect is not misuse, got %v", err)
	}
}

// TestEffect_CallbackThatSummarizesItsOwnTaskKeepsTheEffect guards the
// fix's blast radius: `Summary(text)` inside an Effect callback sets result
// metadata without resolving the Task, and the effect still records.
func TestEffect_CallbackThatSummarizesItsOwnTaskKeepsTheEffect(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := initRun(Config{title: "setup", stdout: &buf, plain: true, color: colorNever})

	pkg := out.Task("numpy")
	pkg.Define(func(ctx context.Context) error {
		return Effect(ctx, createModule, func(context.Context) error {
			pkg.Summary("installed from cache")
			return nil
		})
	})
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	if got := buf.String(); !strings.Contains(got, "created 1 module") {
		t.Fatalf("a Summary inside the callback must keep its effect:\n%s", got)
	}
}

// createModule is the one-module Effect these verdict tests share.
var createModule = EffectSpec{Verb: Create, Object: "module", Quantity: 1}

// TestEffect_ConcurrentEffectsBothHonorTheRowsDenial pins that the denial
// verdict belongs to every Effect in flight when the row disowns its work,
// not to whichever Effect happens to exit first: a second Effect running
// concurrently in the same Define once saw the first one's exit reset the
// shared flag and recorded work its row had disowned.
func TestEffect_ConcurrentEffectsBothHonorTheRowsDenial(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := initRun(Config{title: "setup", stdout: &buf, plain: true, color: colorNever})

	broken := out.Task("broken")
	broken.Define(func(ctx context.Context) error {
		entered := make(chan struct{})
		release := make(chan struct{})
		second := make(chan error, 1)
		go func() {
			second <- Effect(ctx, EffectSpec{Verb: Create, Object: "cache", Quantity: 1}, func(context.Context) error {
				close(entered)
				<-release
				return nil
			})
		}()
		first := Effect(ctx, createModule, func(context.Context) error {
			<-entered
			broken.fail("uv rejected the package")
			return nil
		})
		close(release)
		if err := <-second; err != nil {
			return err
		}
		return first
	})
	if err := out.finish(); err != nil {
		t.Log(err)
	}
	if got := buf.String(); strings.Contains(got, "[changed]") {
		t.Fatalf("no Effect may record work its row disowned:\n%s", got)
	}
}

// TestEffect_SuccessCommitsChangedEffect proves the ordinary
// path: call executes, succeeds, and the effect commits into the Changes
// ledger — evo derives StateChanged, the caller never chose it.
func TestEffect_SuccessCommitsChangedEffect(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true})

	branches := out.Task("branches")
	branches.Define(effectOf(Delete, "stale local branch", 2))

	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if got := out.conclusion().headline(); got != headlineChanged {
		t.Fatalf("state = %v, want StateChanged", got)
	}
	if !strings.Contains(buf.String(), "deleted 2 stale local branches") {
		t.Fatalf("want the derived past-tense ledger row, got:\n%s", buf.String())
	}
}

// TestEffect_NilCallRecordsWithoutExecuting proves call == nil
// still commits the effect (there is nothing to execute, so nothing can
// fail) on a normal run.
func TestEffect_NilCallRecordsWithoutExecuting(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true})

	branches := out.Task("branches")
	branches.Define(effectOf(Delete, "stale local branch", 2))

	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if got := out.conclusion().headline(); got != headlineChanged {
		t.Fatalf("state = %v, want StateChanged", got)
	}
}

// TestEffect_CallErrorCommitsNothing proves a failing call
// commits no effect and returns the error verbatim — the caller decides
// Fail/Block from there, evo never guesses.
func TestEffect_CallErrorCommitsNothing(t *testing.T) {
	out := initRun(Config{stdout: io.Discard, color: colorNever, plain: true})

	branches := out.Task("branches")
	wantErr := errors.New("permission denied")
	branches.fail("delete stale branches", Detail(wantErr.Error()))

	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	snap := out.snapshot()
	for _, ch := range snap.Changes {
		if len(ch.Records) > 0 {
			t.Fatalf("want no committed records after a call error, got %+v", ch.Records)
		}
	}
	if got := out.conclusion().headline(); got != headlineFailed {
		t.Fatalf("state = %v, want StateFailed", got)
	}
}

// TestEffect_DryRunNeverExecutesCallAndPlansEffect proves the
// dry-run half: call is never invoked, and the effect is recorded as
// planned, never changed.
func TestEffect_DryRunNeverExecutesCallAndPlansEffect(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true, DryRun: true})

	branches := out.Task("branches")
	called := false
	branches.Define(func(ctx context.Context) error {
		return Effect(ctx, EffectSpec{Verb: Delete, Object: "stale local branch", Quantity: 2}, func(context.Context) error {
			called = true
			return nil
		})
	})

	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("dry-run must not invoke the mutation callback")
	}
	if got := out.conclusion().headline(); got != headlinePlanned {
		t.Fatalf("state = %v, want StatePlanned", got)
	}
	if !strings.Contains(buf.String(), "delete 2 stale local branches") {
		t.Fatalf("want the imperative planned ledger row, got:\n%s", buf.String())
	}
}
