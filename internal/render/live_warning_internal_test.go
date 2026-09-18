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
	writeLiveTaskLine(&b, snap, 1, 0, 80, "⠋", false, time.Time{}, txt.GlyphsUnicode)
	got := b.String()
	if !strings.Contains(got, "✗ write launch agent  failed: permissions") {
		t.Fatalf("missing failed parent:\n%s", got)
	}
	if !strings.Contains(got, "! chmod denied") {
		t.Fatalf("Failed warning must nest under the parent:\n%s", got)
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
	writeLiveTaskLine(&b, snap, 1, 0, 80, "⠋", false, time.Time{}, txt.GlyphsUnicode)
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
	got := LiveRegion(snap, 24, 80, time.Time{}, false, txt.GlyphsUnicode)
	if !strings.Contains(got, "[changed] branches") || !strings.Contains(got, "deleted 5 local tips") {
		t.Fatalf("missing [changed] ledger:\n%s", got)
	}
	if !strings.Contains(got, "[planned] remote-tracking") || !strings.Contains(got, "fetch-prune 12 stale origin/*") {
		t.Fatalf("missing [planned] ledger:\n%s", got)
	}
}
