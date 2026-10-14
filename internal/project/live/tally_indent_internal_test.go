package live

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/project/plain"

	"github.com/zachbornheimer/evident-output/v2/internal/project"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// branchesWithWorkChild is a Group with its own Summary, four Excluded
// items that fold into one tally, and one work child whose row survives
// beside it.
func branchesWithWorkChild(work record.TaskSnapshot) record.TasksSnapshot {
	return record.TasksSnapshot{Name: "branches", State: record.Succeeded, Summary: "14 deleted", Tasks: []record.TaskSnapshot{
		excludedItem("main", "protected"),
		excludedItem("develop", "protected"),
		excludedItem("feature-a", "unpushed"),
		excludedItem("feature-b", "unpushed"),
		work,
	}}
}

// TestGroupTallies_ShareTheChildColumn: under a Group header, the folded
// tally is the header's child like the rows that survive, so both start in
// the same column.
func TestGroupTallies_ShareTheChildColumn(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	plain.WriteCollection(&b, branchesWithWorkChild(record.TaskSnapshot{Name: "deleted", State: record.Succeeded, Summary: "14 deleted"}), project.Style{Profile: project.GlyphsUnicode})
	want := "✓ branches  14 deleted\n" +
		"   ○ excluded 4 (2 protected, 2 unpushed)\n" +
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
	if len(lines) != 3 {
		t.Fatalf("live frame: want header, one tally, one child:\n%s", b.String())
	}
	for _, line := range lines[1:] {
		if !strings.HasPrefix(line, project.ProblemTreeIndent) || strings.HasPrefix(line, project.ProblemTreeIndent+" ") {
			t.Fatalf("live row %q is not in the child column:\n%s", line, b.String())
		}
	}
}

// writeLiveCollection writes col's live rows within height and reports how
// many it wrote. It exceeds height only below a Group's floor (see
// minLiveChildRows).
func writeLiveCollection(b *strings.Builder, col record.TasksSnapshot, height int, st liveStyle) (rows int) {
	return writeAlignedLiveCollection(b, col, height, 0, st)
}
