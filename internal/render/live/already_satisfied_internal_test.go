package live

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

func TestAlreadySatisfiedRowDetail_LiveUnit(t *testing.T) {
	t.Parallel()
	snap := core.TaskSnapshot{
		Name:       "services",
		State:      core.Done,
		Summary:    "nope",
		Resolution: core.ResolutionAlreadySatisfied,
	}
	unit := liveTaskUnit(snap, 1, countWidths{}, liveStyle{Style: render.Style{Profile: txt.GlyphsUnicode}, width: 80, spin: "⠋"})
	if unit.Detail != render.AlreadySatisfiedDetail {
		t.Fatalf("live Detail = %q, want %q", unit.Detail, render.AlreadySatisfiedDetail)
	}
}
