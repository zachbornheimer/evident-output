package evo_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// collapse folds runs of whitespace into single spaces, so an assertion
// against wrapped/indented durable output does not depend on exact column
// widths.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// mutatedThenFailed runs one mutation per subject and then fails the run, so
// the conclusion renders its "! already mutated: ..." line over a ledger the
// test controls exactly.
func mutatedThenFailed(t *testing.T, mutate func(*evo.Output)) string {
	t.Helper()
	var buf bytes.Buffer
	out := isolatedScheduler(t, 1, &buf, false)
	mutate(out)
	out.Task("verify").Define(func(ctx context.Context) error { return errProbe })
	_ = out.Finish()
	return collapse(buf.String())
}

// TestConclusion_P7_AlreadyMutatedAggregatesAndNamesItsOwner pins the P7 /
// axis-16 complaint: "! already mutated: 1 module created; 1 module created"
// names no task and repeats itself. A fragment owned by a single task says
// which. (The identical-effects-across-tasks aggregation this test used to
// also cover ran through Each's fromEach→collection-name subject folding
// (ledgerSubjectFor) — Each was removed outright in 1.0 (§3.1: get-or-create
// reliance is unsound), so two plain Group children now each own their own
// "already mutated" fragment instead of folding into the collection's.)
func TestConclusion_P7_AlreadyMutatedAggregatesAndNamesItsOwner(t *testing.T) {
	t.Parallel()

	t.Run("distinct effects name the task that owns each", func(t *testing.T) {
		t.Parallel()
		got := mutatedThenFailed(t, func(out *evo.Output) {
			out.Task("modules").Define(effectOf(evo.EffectCreate, "module", 1))
			out.Task("records").Define(effectOf(evo.EffectUpdate, "record", 1))
		})
		if !strings.Contains(got, "modules: 1 module created") {
			t.Fatalf("want the owning task named, got:\n%s", got)
		}
		if !strings.Contains(got, "records: 1 record updated") {
			t.Fatalf("want the owning task named, got:\n%s", got)
		}
	})
}

// TestConclusion_AlreadyMutated_CancelledWithChanges is red-first for item 1:
// a Cancelled run with committed effects must render one derived
// "! partial changes were applied before cancellation" note (contract §15),
// never a caller-assembled string and never a second copy of the ledger.
func TestConclusion_AlreadyMutated_CancelledWithChanges(t *testing.T) {
	var buf strings.Builder
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	branches := out.Task("branches")
	branches.Define(effectOf(evo.EffectDelete, "local branch", 8))
	out.Cancel("interrupted")
	if err := out.Finish(); err != nil {
		t.Log(err)
	}
	got := buf.String()
	if !strings.Contains(got, "! partial changes were applied before cancellation") {
		t.Fatalf("want the partial-changes note, got:\n%s", got)
	}
}

// TestConclusion_AlreadyMutated_CancelledEmptyLedger proves an empty Changes
// ledger suppresses the "! already mutated: ..." row entirely — "!" is
// attention-only (evo-rec.md "Tightened glyph vocabulary"), and "none" earns
// no attention. Partial truth still holds: there is simply nothing to report.
func TestConclusion_AlreadyMutated_CancelledEmptyLedger(t *testing.T) {
	var buf strings.Builder
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	out.Task("scan").Doing("scanning")
	out.Cancel("interrupted")
	if err := out.Finish(); err != nil {
		t.Log(err)
	}
	got := buf.String()
	if strings.Contains(got, "already mutated") {
		t.Fatalf("empty ledger must not render already-mutated line, got:\n%s", got)
	}
}

// TestConclusion_AlreadyMutated_Failed proves the same derived line renders
// on a Failed conclusion, not only Cancelled.
func TestConclusion_AlreadyMutated_Failed(t *testing.T) {
	var buf strings.Builder
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	remotes := out.Task("remotes")
	remotes.Define(func(ctx context.Context) error {
		if err := effectOf(evo.EffectDelete, "origin tip", 1)(ctx); err != nil {
			return err
		}
		return errors.New("authentication failed")
	})
	if err := out.Finish(); err != nil {
		t.Log(err)
	}
	got := buf.String()
	if !strings.Contains(got, "!  already mutated: 1 origin tip deleted") {
		t.Fatalf("want derived already-mutated line on Failed, got:\n%s", got)
	}
}

// TestConclusion_AlreadyMutated_NotRenderedOnSuccess proves the line is
// specific to abnormal termination — a normal Done run never renders it.
func TestConclusion_AlreadyMutated_NotRenderedOnSuccess(t *testing.T) {
	var buf strings.Builder
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	branches := out.Task("branches")
	branches.Define(effectOf(evo.EffectDelete, "local branch", 8))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, "already mutated") {
		t.Fatalf("success must not render already-mutated line, got:\n%s", got)
	}
}
