package evo_test

import (
	"bytes"
	"context"
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
			out.Task("modules").Create("module", func() error { return nil })
			out.Task("files").Write("file", func() error { return nil })
		})
		if !strings.Contains(got, "modules: 1 module created") {
			t.Fatalf("want the owning task named, got:\n%s", got)
		}
		if !strings.Contains(got, "files: 1 file wrote") {
			t.Fatalf("want the owning task named, got:\n%s", got)
		}
	})
}
