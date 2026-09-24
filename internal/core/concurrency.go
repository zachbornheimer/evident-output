package core

import (
	"slices"
	"time"
)

// runningInterval is one Task's half-open [start, end) Running span.
type runningInterval struct{ start, end time.Time }

// peakOverlap is the most intervals open at one instant. Ends sort before
// starts at the same instant, so one Task settling as the next starts is
// back-to-back, not concurrent.
func peakOverlap(intervals []runningInterval) int {
	type edge struct {
		at    time.Time
		delta int
	}
	edges := make([]edge, 0, 2*len(intervals))
	for _, iv := range intervals {
		edges = append(edges, edge{iv.start, 1}, edge{iv.end, -1})
	}
	slices.SortFunc(edges, func(a, b edge) int {
		if c := a.at.Compare(b.at); c != 0 {
			return c
		}
		return a.delta - b.delta
	})
	open, peak := 0, 0
	for _, e := range edges {
		open += e.delta
		peak = max(peak, open)
	}
	return peak
}
