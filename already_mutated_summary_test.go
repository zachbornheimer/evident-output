package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// mutatedThenFailed runs one mutation per subject and then fails the run, so
// the conclusion renders its "! already mutated: ..." line over a ledger the
// test controls exactly.
func mutatedThenFailed(t *testing.T, mutate func(*evo.Output)) string {
	t.Helper()
	var buf bytes.Buffer
	out := isolatedScheduler(t, 1, &buf, false)
	mutate(out)
	out.Task("verify").Define(func() error { return errProbe })
	_ = out.Finish()
	return collapse(buf.String())
}

// TestConclusion_P7_AlreadyMutatedAggregatesAndNamesItsOwner pins the P7 /
// axis-16 complaint: "! already mutated: 1 module created; 1 module created"
// names no task and repeats itself. Identical effects aggregate into one
// counted fragment, and a fragment owned by a single task says which.
func TestConclusion_P7_AlreadyMutatedAggregatesAndNamesItsOwner(t *testing.T) {
	t.Parallel()

	t.Run("identical effects across tasks aggregate", func(t *testing.T) {
		t.Parallel()
		got := mutatedThenFailed(t, func(out *evo.Output) {
			for _, task := range out.Group("setup").Each([]string{"mod-a", "mod-b"}) {
				task.Create("module", func() error { return nil })
			}
		})
		if !strings.Contains(got, "already mutated: 2 modules created") {
			t.Fatalf("want the aggregated fragment, got:\n%s", got)
		}
		if strings.Contains(got, "1 module created; 1 module created") {
			t.Fatalf("duplicate fragments survived:\n%s", got)
		}
	})

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
