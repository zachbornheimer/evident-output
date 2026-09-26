package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// keptItemCount is the benchmark's per-item children: zq prune holds about
// 270 per run; 1k gives the 100ms heartbeat headroom a larger repository needs.
const keptItemCount = 1000

// keptCategory is one prune category at keptItemCount: its still-running
// same-named work Task plus keptItemCount per-item children, each resolved
// Kept under one of three reasons.
func keptCategory(items int) core.TasksSnapshot {
	reasons := []string{"checked out", "unpushed", "protected"}
	tasks := make([]core.TaskSnapshot, 0, items+1)
	tasks = append(tasks, core.TaskSnapshot{Name: "branches", State: core.Running, Phase: "deleting"})
	for i := range items {
		name := fmt.Sprintf("feat/branch-%04d", i)
		tasks = append(tasks, core.TaskSnapshot{
			Name: name, State: core.Done,
			Kept: []core.TaxonomyRecord{{Reason: reasons[i%len(reasons)], Name: name}},
		})
	}
	return core.TasksSnapshot{Name: "branches", State: core.Running, Tasks: tasks}
}

// TestKeptCategory_FoldsEveryItemAndSuppressesTheTallyWhileRunning folds
// every one of the category's items rather than paint keptItemCount rows
// (§25), and — contract §18 — shows none of that fold as a "! kept N"
// tally while the category's own Task is still Running: the count could
// still grow before the Task settles.
func TestKeptCategory_FoldsEveryItemAndSuppressesTheTallyWhileRunning(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	writeLiveCollection(&b, keptCategory(keptItemCount), 40, testLiveStyle)
	got := b.String()
	if strings.Contains(got, "feat/branch-") {
		t.Fatalf("want no per-item rows for %d folded items, got:\n%s", keptItemCount, got)
	}
	if strings.Contains(got, "! kept") {
		t.Fatalf("want no kept tally while the category is still running, got:\n%s", got)
	}
}

// BenchmarkLiveFrame_KeptCategory1k is one live-region frame of a category
// with 1k kept children — the work the 100ms heartbeat repeats.
func BenchmarkLiveFrame_KeptCategory1k(b *testing.B) {
	col := keptCategory(keptItemCount)
	b.ReportAllocs()
	for b.Loop() {
		var sb strings.Builder
		writeLiveCollection(&sb, col, 40, testLiveStyle)
	}
}

// BenchmarkDurable_KeptCategory1k is the durable (plain / final) render of
// the same category, verbose so every item name is listed.
func BenchmarkDurable_KeptCategory1k(b *testing.B) {
	col := keptCategory(keptItemCount)
	b.ReportAllocs()
	for b.Loop() {
		var sb strings.Builder
		WriteCollection(&sb, col, Style{Verbose: true, Profile: txt.GlyphsUnicode})
	}
}
