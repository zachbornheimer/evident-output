package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// skippedItemCount is the benchmark's per-item children: zq prune holds about
// 270 per run; 1k gives the 100ms heartbeat headroom a larger repository needs.
const skippedItemCount = 1000

// skippedCategory is one prune category at skippedItemCount: its still-running
// same-named work Task plus skippedItemCount per-item children, each resolved
// Skipped under one of three reasons.
func skippedCategory(items int) core.TasksSnapshot {
	reasons := []string{"checked out", "unpushed", "protected"}
	tasks := make([]core.TaskSnapshot, 0, items+1)
	tasks = append(tasks, core.TaskSnapshot{Name: "branches", State: core.Running, Phase: "deleting"})
	for i := range items {
		name := fmt.Sprintf("feat/branch-%04d", i)
		tasks = append(tasks, core.TaskSnapshot{
			Name: name, State: core.Skipped,
			Skipped: []core.TaxonomyRecord{{Reason: reasons[i%len(reasons)], Name: name}},
		})
	}
	return core.TasksSnapshot{Name: "branches", State: core.Running, Tasks: tasks}
}

func TestSkippedCategory_FoldsEveryItemIntoOneTally(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	writeLiveCollection(&b, skippedCategory(skippedItemCount), 40, testLiveStyle)
	if got := b.String(); strings.Contains(got, "feat/branch-") || !strings.Contains(got, "- skipped 1000 (334 checked out, 333 unpushed, 333 protected)") {
		t.Fatalf("want one tally for %d items, got:\n%s", skippedItemCount, got)
	}
}

// BenchmarkLiveFrame_SkippedCategory1k is one live-region frame of a category
// with 1k skipped children — the work the 100ms heartbeat repeats.
func BenchmarkLiveFrame_SkippedCategory1k(b *testing.B) {
	col := skippedCategory(skippedItemCount)
	b.ReportAllocs()
	for b.Loop() {
		var sb strings.Builder
		writeLiveCollection(&sb, col, 40, testLiveStyle)
	}
}

// BenchmarkDurable_SkippedCategory1k is the durable (plain / final) render of
// the same category, verbose so every item name is listed.
func BenchmarkDurable_SkippedCategory1k(b *testing.B) {
	col := skippedCategory(skippedItemCount)
	b.ReportAllocs()
	for b.Loop() {
		var sb strings.Builder
		WriteCollection(&sb, col, Style{Verbose: true, Profile: txt.GlyphsUnicode})
	}
}
