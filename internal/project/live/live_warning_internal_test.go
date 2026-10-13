package live

import (
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/project"

	"github.com/zachbornheimer/evident-output/internal/record"
)

func TestWriteLiveTaskLine_FailedWarningNests(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	snap := record.TaskSnapshot{
		Name:     "write launch agent",
		State:    record.Failed,
		Summary:  "failed: permissions",
		Warnings: []record.Problem{{Summary: "chmod denied"}},
	}
	writeLiveTaskLine(&b, snap, 1, 0, testLiveStyle)
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
	snap := record.TaskSnapshot{
		Name:     "prepare hosts",
		State:    record.Running,
		Phase:    "host-031",
		Progress: record.Progress{Kind: record.Determinate, Completed: 31, Total: 100},
	}
	writeLiveTaskLine(&b, snap, 1, 0, testLiveStyle)
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
	snap := record.Snapshot{
		Tasks: []record.TaskSnapshot{{Name: "work", State: record.Succeeded}},
		Changes: []record.ChangesSnapshot{{
			Subject: "branches",
			Records: []record.EffectRecord{{Verb: "deleted", Quantity: 5, HasQty: true, Object: "local tip"}},
		}},
		Plans: []record.PlanSnapshot{{
			Subject: "remote-tracking",
			Records: []record.EffectRecord{{Verb: "fetch-prune", Quantity: 12, HasQty: true, Object: "stale origin/*"}},
		}},
	}
	got := LiveRegion(snap, 24, 80, time.Time{}, project.Style{Profile: project.GlyphsUnicode})
	if !strings.Contains(got, "[changed] branches") || !strings.Contains(got, "deleted 5 local tips") {
		t.Fatalf("missing [changed] ledger:\n%s", got)
	}
	if !strings.Contains(got, "[planned] remote-tracking") || !strings.Contains(got, "fetch-prune 12 stale origin/*") {
		t.Fatalf("missing [planned] ledger:\n%s", got)
	}
}
