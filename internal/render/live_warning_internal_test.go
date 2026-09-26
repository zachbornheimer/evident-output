package render

import (
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

func TestWriteLiveTaskLine_FailedWarningNests(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	snap := core.TaskSnapshot{
		Name:     "write launch agent",
		State:    core.Failed,
		Summary:  "failed: permissions",
		Warnings: []core.Problem{{Summary: "chmod denied"}},
	}
	writeLiveTaskLine(&b, snap, 1, 0, countWidths{}, testLiveStyle)
	got := b.String()
	if !strings.Contains(got, "✗ write launch agent  failed: permissions") {
		t.Fatalf("missing failed parent:\n%s", got)
	}
	if !strings.Contains(got, "! chmod denied") {
		t.Fatalf("Failed warning must nest under the parent:\n%s", got)
	}
}

// TestWriteLiveTaskLine_SkippedTaxonomyNestsWhileRunning proves a
// standalone Running task's own accumulated Skipped taxonomy (recorded via
// TaskHandle.Skipped/SkippedWithErrs on the task itself, not a folded Group
// tally across still-arriving sibling children) nests under its row the
// same way a warning does — it is the task's own already-final
// self-reported information, not a count that could still grow, so it is
// not subject to the Group-fold "don't paint while classifying" rule
// (categoryStillClassifying) that guards a Group's own tally.
func TestWriteLiveTaskLine_SkippedTaxonomyNestsWhileRunning(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	snap := core.TaskSnapshot{
		Name:     "cleanup",
		State:    core.Running,
		Phase:    "feat/cleanup…",
		Progress: core.Progress{Kind: core.Determinate, Completed: 7, Total: 18},
		Skipped: []core.TaxonomyRecord{
			{Reason: "protected", Name: "a"},
			{Reason: "protected", Name: "b"},
			{Reason: "protected", Name: "c"},
			{Reason: "unpushed", Name: "d"},
			{Reason: "unpushed", Name: "e"},
		},
	}
	writeLiveTaskLine(&b, snap, 1, 0, countWidths{}, testLiveStyle)
	got := b.String()
	if !strings.Contains(got, "- skipped 5 (3 protected, 2 unpushed)") {
		t.Fatalf("Running task's own Skipped taxonomy must nest under its row:\n%s", got)
	}
}

func TestWriteLiveTaskLine_GroupChildSplitsActivity(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	snap := core.TaskSnapshot{
		Name:     "prepare hosts",
		State:    core.Running,
		Phase:    "host-031",
		Progress: core.Progress{Kind: core.Determinate, Completed: 31, Total: 100},
	}
	writeLiveTaskLine(&b, snap, 1, 0, countWidths{}, testLiveStyle)
	got := strings.TrimRight(b.String(), "\n")
	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Fatalf("Group-child Running+Progress+Doing must split into parent + activity, got %d line(s):\n%s", len(lines), got)
	}
	if strings.Contains(lines[0], "host-031") {
		t.Fatalf("parent bar row must not carry the activity name:\n%s", got)
	}
	if !strings.Contains(lines[0], "31/100") {
		t.Fatalf("parent bar row must keep the count:\n%s", got)
	}
	if strings.TrimSpace(lines[1]) != "⠋ host-031" {
		t.Fatalf("activity child line = %q, want spinner + host-031:\n%s", lines[1], got)
	}
	if !strings.HasPrefix(lines[1], "      ") {
		t.Fatalf("activity child under a Group child must indent six spaces, got %q", lines[1])
	}
}

func TestLiveRegion_ProjectsChangedAndPlannedLedger(t *testing.T) {
	t.Parallel()
	snap := core.Snapshot{
		Tasks: []core.TaskSnapshot{{Name: "work", State: core.Done}},
		Changes: []core.ChangesSnapshot{{
			Subject: "branches",
			Records: []core.EffectRecord{{Verb: "deleted", Quantity: 5, HasQty: true, Object: "local tip"}},
		}},
		Plans: []core.PlanSnapshot{{
			Subject: "remote-tracking",
			Records: []core.EffectRecord{{Verb: "fetch-prune", Quantity: 12, HasQty: true, Object: "stale origin/*"}},
		}},
	}
	got := LiveRegion(snap, 24, 80, time.Time{}, Style{Profile: txt.GlyphsUnicode})
	if !strings.Contains(got, "[changed] branches") || !strings.Contains(got, "deleted 5 local tips") {
		t.Fatalf("missing [changed] ledger:\n%s", got)
	}
	if !strings.Contains(got, "[planned] remote-tracking") || !strings.Contains(got, "fetch-prune 12 stale origin/*") {
		t.Fatalf("missing [planned] ledger:\n%s", got)
	}
}
