package core

import (
	"slices"
	"time"
)

// runningInterval is one Task's half-open [start, end) Running span.
type runningInterval struct{ start, end time.Time }

// runningIntervals collects the Running spans of the Tasks a walk visits.
type runningIntervals []runningInterval

// add records timing's Running span, if the Task ran.
func (r *runningIntervals) add(timing TaskTiming) {
	if timing.Running() > 0 {
		*r = append(*r, runningInterval{timing.StartedAt, timing.SettledAt})
	}
}

// peak is the most recorded spans open at one instant.
func (r runningIntervals) peak() int { return peakOverlap(r) }

// PeakConcurrency is the most of this Group's or Sequence's Tasks, nested
// collections included, that were Running at one instant: the §39 group
// concurrency. A Sequence reads at most 1 unless a step ran inline. Tasks
// outside the collection never count, unlike RunMetrics.PeakConcurrency.
func (c TasksSnapshot) PeakConcurrency() int {
	var running runningIntervals
	walkTasks(c.Tasks, c.Collections, func(t TaskSnapshot) { running.add(t.Timing) })
	return running.peak()
}

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
