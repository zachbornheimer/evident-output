package live

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/project/plain"

	"github.com/zachbornheimer/evident-output/internal/project"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// branchesWithWorkChild is a Group with its own Summary, two Skipped and
// two Kept items that fold into tallies, and one work child whose row
// survives beside them.
func branchesWithWorkChild(work record.TaskSnapshot) record.TasksSnapshot {
	protected := []record.TaxonomyRecord{{Reason: "protected"}}
	unpushed := []record.TaxonomyRecord{{Reason: "unpushed"}}
	return record.TasksSnapshot{Name: "branches", State: record.Done, Summary: "14 deleted", Tasks: []record.TaskSnapshot{
		{Name: "main", State: record.Skipped, Skipped: protected},
		{Name: "develop", State: record.Skipped, Skipped: protected},
		{Name: "feature-a", State: record.Done, Kept: unpushed},
		{Name: "feature-b", State: record.Done, Kept: unpushed},
		work,
	}}
}

// TestGroupTallies_ShareTheChildColumn: under a Group header, folded
// tallies are the header's children like the rows that survive, so both
// start in the same column.
func TestGroupTallies_ShareTheChildColumn(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	plain.WriteCollection(&b, branchesWithWorkChild(record.TaskSnapshot{Name: "deleted", State: record.Done, Summary: "14 deleted"}), project.Style{Profile: project.GlyphsUnicode})
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
	col := branchesWithWorkChild(record.TaskSnapshot{Name: "deleting", State: record.Running})
	col.State = record.Running
	var b strings.Builder
	writeLiveCollection(&b, col, 20, testLiveStyle)
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("live frame: want header, two tallies, one child:\n%s", b.String())
	}
	for _, line := range lines[1:] {
		if !strings.HasPrefix(line, project.ProblemTreeIndent) || strings.HasPrefix(line, project.ProblemTreeIndent+" ") {
			t.Fatalf("live row %q is not in the child column:\n%s", line, b.String())
		}
	}
}
