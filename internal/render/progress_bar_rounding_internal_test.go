package render

import "testing"

// TestProgressBar_RoundingTable is the RED-then-GREEN regression for
// contract §18's normative rounding table: a naive nearest-value rounding
// of completed/total*width would round 120/459 on a 12-cell bar down to
// 3 filled cells, contradicting §18's own cited frame ("120/459 -> 4/12
// filled, 70/294 -> 3/12 filled"). progressBar must instead treat any
// nonzero fraction of a cell as that cell started (ceiling), clamp a
// nonzero completed count up to at least 1 filled cell even when the
// fraction rounds to 0, and only show the bar as full when completed
// equals total.
func TestProgressBar_RoundingTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name             string
		completed, total int64
		width            int
		wantFilled       int
	}{
		{"§18 worked example: 120/459", 120, 459, 12, 4},
		{"§18 worked example: 70/294", 70, 294, 12, 3},
		{"§18 worked example: 1/4 exact", 1, 4, 12, 3},
		{"tiny nonzero fraction still shows 1 filled cell", 1, 1000, 12, 1},
		{"never shown as full before completion", 999, 1000, 12, 11},
		{"full only at completed == total", 1000, 1000, 12, 12},
		{"zero completed shows zero filled", 0, 100, 12, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := countFilledCells(progressBar(tc.completed, tc.total, tc.width))
			if got != tc.wantFilled {
				t.Fatalf("progressBar(%d, %d, %d): got %d filled cells, want %d",
					tc.completed, tc.total, tc.width, got, tc.wantFilled)
			}
		})
	}
}

// countFilledCells counts the "█" glyphs progressBar renders between its
// brackets, so this test pins the rounding table without duplicating
// progressBar's own bracket/space layout.
func countFilledCells(bar string) int {
	n := 0
	for _, r := range bar {
		if r == '█' {
			n++
		}
	}
	return n
}
