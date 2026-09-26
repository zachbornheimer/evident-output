package render

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// TestConclusionColor_PlannedIsBlue reproduces gate-7 finding 3: StatePlanned
// fell through conclusionColor's default branch (SGRCyan, the "unknown
// state" fallback) instead of naming its own color. conclusionColor is
// already the one shared function both band-rendering call sites use
// (plain.go's final report and progressive.go's live-final band) — this
// closes the gap by naming StatePlanned's color explicitly, so the next new
// ConclusionState can't silently diverge between the two sites either.
func TestConclusionColor_PlannedIsBlue(t *testing.T) {
	if got := conclusionColor(core.StatePlanned); got != txt.SGRBlue {
		t.Fatalf("conclusionColor(StatePlanned) = %q, want SGRBlue (%q)", got, txt.SGRBlue)
	}
}
