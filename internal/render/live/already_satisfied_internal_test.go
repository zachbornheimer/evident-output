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
	style := liveStyle{Style: render.Style{Profile: txt.GlyphsUnicode}, width: 80, spin: "⠋"}
	unit := liveTaskUnit(snap, 1, countWidths{}, style)
	want := render.AlreadySatisfiedRowDetail(snap, style.Color)
	if unit.Detail != want {
		t.Fatalf("live Detail = %q, want %q", unit.Detail, want)
	}
}
