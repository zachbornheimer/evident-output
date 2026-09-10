package evo

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/render"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// TestFitLiveRegion_UnderWidthReturnsInputUnchanged is the root-package
// gate for the FitLiveRegion fast path: multi-line text that already fits
// returns the input byte-identical (no Split/Join rewrite). Over-width
// lines are still truncated to the column budget.
func TestFitLiveRegion_UnderWidthReturnsInputUnchanged(t *testing.T) {
	const columns = 40
	under := "short line\nanother short\nthird"
	got := render.FitLiveRegion(under, columns)
	if got != under {
		t.Fatalf("under-width fit rewrote input:\nwant %q\ngot  %q", under, got)
	}
	if !strings.Contains(got, "\n") {
		t.Fatal("under-width fit dropped embedded newlines")
	}

	over := strings.Repeat("x", columns+20)
	fitted := render.FitLiveRegion(over, columns)
	if fitted == over {
		t.Fatal("over-width line was not truncated")
	}
	if cells := txt.VisibleCells(fitted); cells > columns {
		t.Fatalf("truncated line uses %d cells, budget %d: %q", cells, columns, fitted)
	}
	if !strings.HasSuffix(txt.StripANSI(fitted), "…") {
		t.Fatalf("truncated line must end with ellipsis: %q", fitted)
	}
}
