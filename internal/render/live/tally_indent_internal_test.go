package live

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/render/plain"

	"github.com/zachbornheimer/evident-output/internal/render"

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
	plain.WriteCollection(&b, branchesWithWorkChild(core.TaskSnapshot{Name: "deleted", State: core.Done, Summary: "14 deleted"}), render.Style{Profile: txt.GlyphsUnicode})
	want := "✓ branches  14 deleted\n" +
		"   - skipped 2 (protected)\n" +
		"   ! kept 2 (unpushed)\n" +
		"   ✓ deleted  14 deleted\n"
	if got := b.String(); got != want {
		t.Fatalf("durable group:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestLiveGroupTallies_ShareTheChildColumn is the live frame's same rule.
func TestLiveGroupTallies_ShareTheChildColumn(t *testing.T) {
	t.Parallel()
	col := branchesWithWorkChild(core.TaskSnapshot{Name: "deleting", State: core.Running})
	col.State = core.Running
	var b strings.Builder
	writeLiveCollection(&b, col, 20, testLiveStyle)
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("live frame: want header, two tallies, one child:\n%s", b.String())
	}
	for _, line := range lines[1:] {
		if !strings.HasPrefix(line, render.ProblemTreeIndent) || strings.HasPrefix(line, render.ProblemTreeIndent+" ") {
			t.Fatalf("live row %q is not in the child column:\n%s", line, b.String())
		}
	}
}
