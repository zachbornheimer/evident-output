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
