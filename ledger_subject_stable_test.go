package evo

import (
	"context"
	"strings"
	"testing"
)

// TestLedgerRowSubjectIsStableAcrossRuns pins that a ledger row names the
// same subject for the same mutation on every run. The deletes are the
// "prune" Group's; sibling "bundle" writes a file on the first run and has
// nothing to do on the second. The row used to move up to the "ship"
// Sequence once bundle stopped changing, naming a phase that did not do the
// deleting. A row lifts to a container only when every member of that
// container contributed to it.
func TestLedgerRowSubjectIsStableAcrossRuns(t *testing.T) {
	t.Chdir(t.TempDir())
	state := State{Dir: t.TempDir()}
	declare := func(o *Run) {
		ship := o.Sequence("ship")
		bundle := ship.Task("bundle")
		bundle.Define(func(ctx context.Context) error {
			return File(ctx, FileSpec{Path: "bundle.txt", Contents: []byte("bundle\n")})
		})
		prune := ship.Group("prune")
		prune.After(bundle)
		for _, name := range []string{"prune a", "prune b"} {
			prune.Task(name).Define(effectOf(Delete, "stale copy", 1))
		}
	}
	for run := 1; run <= 3; run++ {
		got, _ := runPlain(t, Config{State: state}, declare)
		if subject := ledgerSubjectOf(got, "deleted 2 stale copies"); subject != "prune" {
			t.Fatalf("run %d: the deletes are named for %q, want \"prune\":\n%s", run, subject, got)
		}
	}
}

// ledgerSubjectOf is the subject of the [changed] row stating what.
func ledgerSubjectOf(rendered, what string) string {
	for line := range strings.SplitSeq(rendered, "\n") {
		if rest, ok := strings.CutPrefix(line, "[changed] "); ok && strings.HasSuffix(rest, what) {
			return strings.Fields(strings.TrimSuffix(rest, what))[0]
		}
	}
	return ""
}
