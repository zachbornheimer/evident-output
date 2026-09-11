package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestMutation_CallbackThatSkippedItsOwnTaskRecordsNoEffect is the red-first
// proof for the canary's `create-skipped` probe: a mutation callback that
// resolved its own task as Skipped and returned nil still put a `[changed]`
// row in the ledger, so the ledger counted the package the installer had
// just rejected. A nil return after the callback said "skipped" means "I
// handled it", not "I did it"; the effect ledger must not count what the row
// itself denies.
func TestMutation_CallbackThatSkippedItsOwnTaskRecordsNoEffect(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Title: "setup", Stdout: &buf, Plain: true, Color: evo.ColorNever})

	install := out.Group("install")
	broken := install.Task("broken")
	broken.Create("module", func() error {
		broken.Skipped(evo.Reason("install failed"))
		return nil
	})
	ok := install.Task("ok")
	ok.Create("module", func() error { return nil })
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

// TestMutation_CallbackThatFailedItsOwnTaskRecordsNoEffect is the same rule
// on the other terminal verdict: a callback that called Fail on its own task
// and then returned nil claims no mutation.
func TestMutation_CallbackThatFailedItsOwnTaskRecordsNoEffect(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Title: "setup", Stdout: &buf, Plain: true, Color: evo.ColorNever})

	broken := out.Task("broken")
	broken.Create("module", func() error {
		broken.Fail("uv rejected the package", evo.Detail("resolution impossible"))
		return nil
	})
	if err := out.Finish(); err != nil {
		t.Log(err)
	}

	if got := buf.String(); strings.Contains(got, "[changed]") {
		t.Fatalf("a failed callback must record no effect:\n%s", got)
	}
}

// TestMutation_CallbackThatDoneItsOwnTaskKeepsTheEffect guards the fix's
// blast radius: `Done(summary)` inside a mutation callback is the ratified
// proposal shape the dialect teaches, and it still records the effect.
func TestMutation_CallbackThatDoneItsOwnTaskKeepsTheEffect(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Title: "setup", Stdout: &buf, Plain: true, Color: evo.ColorNever})

	pkg := out.Task("numpy")
	pkg.Create("module", func() error {
		pkg.Done("installed from cache")
		return nil
	})
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	if got := buf.String(); !strings.Contains(got, "created 1 module") {
		t.Fatalf("a ratified Done proposal must keep its effect:\n%s", got)
	}
}
