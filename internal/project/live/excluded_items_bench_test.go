package live

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/project/plain"

	"github.com/zachbornheimer/evident-output/v2/internal/project"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// excludedItemCount is the benchmark's per-item children: zq prune holds about
// 270 per run; 1k gives the 100ms heartbeat headroom a larger repository needs.
const excludedItemCount = 1000

// excludedCategory is one prune category at excludedItemCount: its still-running
// same-named work Task plus excludedItemCount per-item children, each
// Excluded under one of three reasons.
func excludedCategory(items int) record.TasksSnapshot {
	reasons := []string{"checked out", "unpushed", "protected"}
	tasks := make([]record.TaskSnapshot, 0, items+1)
	tasks = append(tasks, record.TaskSnapshot{Name: "branches", State: record.Running, Phase: "deleting"})
	for i := range items {
		name := fmt.Sprintf("feat/branch-%04d", i)
		tasks = append(tasks, excludedItem(name, reasons[i%len(reasons)]))
	}
	return record.TasksSnapshot{Name: "branches", State: record.Running, Tasks: tasks}
}

func TestExcludedCategory_FoldsEveryItemIntoOneTally(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	writeLiveCollection(&b, excludedCategory(excludedItemCount), 40, testLiveStyle)
	if got := b.String(); strings.Contains(got, "feat/branch-") || !strings.Contains(got, "○ excluded 1000 (334 checked out, 333 unpushed, 333 protected)") {
		t.Fatalf("want one tally for %d items, got:\n%s", excludedItemCount, got)
	}
}

// BenchmarkLiveFrame_ExcludedCategory1k is one live-region frame of a category
// with 1k Excluded children — the work the 100ms heartbeat repeats.
func BenchmarkLiveFrame_ExcludedCategory1k(b *testing.B) {
	col := excludedCategory(excludedItemCount)
	b.ReportAllocs()
	for b.Loop() {
		var sb strings.Builder
		writeLiveCollection(&sb, col, 40, testLiveStyle)
	}
}

// BenchmarkDurable_ExcludedCategory1k is the durable (plain / final) render of
// the same category, verbose so every item name is listed.
func BenchmarkDurable_ExcludedCategory1k(b *testing.B) {
	col := excludedCategory(excludedItemCount)
	b.ReportAllocs()
	for b.Loop() {
		var sb strings.Builder
		plain.WriteCollection(&sb, col, project.Style{Verbose: true, Profile: project.GlyphsUnicode})
	}
}
