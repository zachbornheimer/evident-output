package render

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// branchesWithWorkChild is a Group with its own Summary, two Skipped and
// two Kept items that fold into tallies, and one work child whose row
// survives beside them.
func branchesWithWorkChild(work core.TaskSnapshot) core.TasksSnapshot {
	protected := []core.TaxonomyRecord{{Reason: "protected"}}
	unpushed := []core.TaxonomyRecord{{Reason: "unpushed"}}
	return core.TasksSnapshot{Name: "branches", State: core.Done, Summary: "14 deleted", Tasks: []core.TaskSnapshot{
		{Name: "main", State: core.Skipped, Skipped: protected},
		{Name: "develop", State: core.Skipped, Skipped: protected},
		{Name: "feature-a", State: core.Done, Kept: unpushed},
		{Name: "feature-b", State: core.Done, Kept: unpushed},
		work,
	}}
}

// TestGroupTallies_ShareTheChildColumn: under a Group header, folded
// tallies are the header's children like the rows that survive, so both
// start in the same column.
func TestGroupTallies_ShareTheChildColumn(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	WriteCollection(&b, branchesWithWorkChild(core.TaskSnapshot{Name: "deleted", State: core.Done, Summary: "14 deleted"}), Style{Profile: txt.GlyphsUnicode})
	want := "✓ branches  14 deleted\n" +
		"   - skipped 2 (protected)\n" +
		"   ! kept 2 (unpushed)\n" +
		"   ✓ deleted  14 deleted\n"
	if got := b.String(); got != want {
		t.Fatalf("durable group:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestLiveGroupTallies_ShareTheChildColumn is the live frame's same rule,
// once the category has settled: contract §18 suppresses the folded
// tallies entirely while any Task in the category is still Running (see
// TestLiveGroupTallies_SuppressTalliesWhileAWorkChildRuns below), so a
// column-alignment assertion over live tally rows needs a settled group.
func TestLiveGroupTallies_ShareTheChildColumn(t *testing.T) {
	t.Parallel()
	col := branchesWithWorkChild(core.TaskSnapshot{Name: "deleted", State: core.Done, Summary: "14 deleted"})
	var b strings.Builder
	writeLiveCollection(&b, col, 20, testLiveStyle)
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("live frame: want header, two tallies, one child:\n%s", b.String())
	}
	for _, line := range lines[1:] {
		if !strings.HasPrefix(line, problemTreeIndent) || strings.HasPrefix(line, problemTreeIndent+" ") {
			t.Fatalf("live row %q is not in the child column:\n%s", line, b.String())
		}
	}
}

// TestLiveGroupTallies_SuppressTalliesWhileAWorkChildRuns is this slice's
// RED-then-GREEN case for the header/default live shape (contract §18,
// extended): a Group whose header does not collapse into its own Task or
// a promoted lone child (own_task.go) — here two Skipped, two Kept and one
// still-Running work child — must not paint its folded tallies while that
// work child is Running: more items could still resolve Skipped or Kept
// before it settles, so the count would understate.
func TestLiveGroupTallies_SuppressTalliesWhileAWorkChildRuns(t *testing.T) {
	t.Parallel()
	col := branchesWithWorkChild(core.TaskSnapshot{Name: "deleting", State: core.Running})
	var b strings.Builder
	writeLiveCollection(&b, col, 20, testLiveStyle)
	frame := b.String()
	for _, unwanted := range []string{"- skipped", "! kept"} {
		if strings.Contains(frame, unwanted) {
			t.Fatalf("live frame must not show a tally while a work child is Running:\n%s", frame)
		}
	}
	if !strings.Contains(frame, "deleting") {
		t.Fatalf("live frame lacks the running work child:\n%s", frame)
	}
}
