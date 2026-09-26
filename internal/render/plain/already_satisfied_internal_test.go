package plain

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// TestAlreadySatisfiedRowDetail_IgnoresSummary is the renderer-level red
// for spec §19: ResolutionAlreadySatisfied owns the muted "already
// satisfied" suffix. A non-empty Summary must not replace it — runDefine
// currently leaves Summary empty, but the suffix is the resolution, not
// whatever text the caller happened to stash.
func TestAlreadySatisfiedRowDetail_IgnoresSummary(t *testing.T) {
	t.Parallel()
	snap := core.TaskSnapshot{
		Name:       "write launch agent",
		State:      core.Done,
		Summary:    "nope",
		Resolution: core.ResolutionAlreadySatisfied,
	}
	var b strings.Builder
	WriteTaskAligned(&b, snap, 0, render.Style{Profile: txt.GlyphsUnicode})
	got := b.String()
	if !strings.Contains(got, "already satisfied") {
		t.Fatalf("missing ResolutionAlreadySatisfied suffix:\n%s", got)
	}
	if strings.Contains(got, "nope") {
		t.Fatalf("Summary replaced the §19 suffix:\n%s", got)
	}
}
