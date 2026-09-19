package render

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

func TestPlain_DryRunHeaderUsesTitleWhenSubjectEmpty(t *testing.T) {
	t.Parallel()
	snap := core.Snapshot{
		Subject: "retire",
		DryRun:  true,
		Plans: []core.PlanSnapshot{{
			Subject: "branches",
			Records: []core.EffectRecord{
				{Verb: "delete", Quantity: 2, HasQty: true, Object: "stale branch"},
			},
		}},
		Conclusion: &core.Conclusion{State: core.StatePlanned, Subject: "retire"},
	}
	got := Plain(snap, 0, true, false, txt.GlyphsASCII)
	if !strings.Contains(got, "[dry-run] retire") {
		t.Fatalf("Title must appear on the dry-run header when Subject is empty, got:\n%s", got)
	}
	if strings.Contains(got, dryRunMarkerText) {
		t.Fatalf("generic dry-run marker must not replace Title, got:\n%s", got)
	}
	if strings.Contains(got, "[planned] retire") {
		t.Fatalf("must not restore a content-free title conclusion band, got:\n%s", got)
	}
}

func TestPlain_DryRunHeaderPrefersSubjectOverTitle(t *testing.T) {
	t.Parallel()
	snap := core.Snapshot{
		Subject:       "retire",
		DryRunSubject: "repo  /tmp/flight",
		DryRun:        true,
	}
	got := Plain(snap, 0, true, false, txt.GlyphsASCII)
	if !strings.HasPrefix(got, "[dry-run] repo  /tmp/flight\n") {
		t.Fatalf("Subject must win over Title, got:\n%s", got)
	}
	if strings.Contains(got, "retire") {
		t.Fatalf("Title must not appear when Subject is set, got:\n%s", got)
	}
}

func TestPlain_DryRunHeaderFallsBackWhenSubjectAndTitleEmpty(t *testing.T) {
	t.Parallel()
	snap := core.Snapshot{DryRun: true}
	got := Plain(snap, 0, true, false, txt.GlyphsASCII)
	if !strings.HasPrefix(got, "[dry-run] "+dryRunMarkerText+"\n") {
		t.Fatalf("empty Subject and Title must keep the generic marker, got:\n%s", got)
	}
}
