package live

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/project"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// TestAlreadySatisfiedRowDetail_IgnoresSummary is the renderer-level red
// for spec §19: ResolutionAlreadySatisfied owns the muted "already
// satisfied" suffix. A non-empty Summary must not replace it — runDefine
// currently leaves Summary empty, but the suffix is the resolution, not
// whatever text the caller happened to stash.
func TestAlreadySatisfiedRowDetail_IgnoresSummary(t *testing.T) {
	t.Parallel()
	snap := record.TaskSnapshot{
		Name:    "write launch agent",
		State:   record.Satisfied,
		Summary: "nope",
	}
	var b strings.Builder
	project.WriteTaskAligned(&b, snap, 0, project.Style{Profile: project.GlyphsUnicode})
	got := b.String()
	if !strings.Contains(got, "already satisfied") {
		t.Fatalf("missing ResolutionAlreadySatisfied suffix:\n%s", got)
	}
	if strings.Contains(got, "nope") {
		t.Fatalf("Summary replaced the §19 suffix:\n%s", got)
	}
}

func TestAlreadySatisfiedRowDetail_LiveUnit(t *testing.T) {
	t.Parallel()
	snap := record.TaskSnapshot{
		Name:    "services",
		State:   record.Satisfied,
		Summary: "nope",
	}
	unit := liveTaskUnit(snap, 1, liveStyle{Style: project.Style{Profile: project.GlyphsUnicode}, width: 80, spin: "⠋"})
	if unit.Detail != project.AlreadySatisfiedDetail {
		t.Fatalf("live Detail = %q, want %q", unit.Detail, project.AlreadySatisfiedDetail)
	}
}
