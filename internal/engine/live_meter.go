package engine

import "time"

// renderBudget bounds the render work forced paints may spend in one
// frame interval. A forced paint renders the whole live region — every
// Task, not only the rows on screen — so forcing one per scheduler
// transition cost O(n) per Task and O(n²) per run (4000 Tasks: 46.9s).
// Under the budget every transition still paints, which is what a small
// run and FP-005's spinner-before-check need; past it, forced paints
// coalesce under the frame-rate cap and the animator paints the latest
// state on its next tick.
type renderBudget struct {
	since time.Time
	spent int
}

// forcedRenderRowsPerInterval is the budget, in Task rows rendered, forced
// paints may spend per frame interval.
const forcedRenderRowsPerInterval = 4096

// allow reports whether a forced paint of rows fits the budget at now, and
// charges it when it does.
func (b *renderBudget) allow(now time.Time, interval time.Duration, rows int) bool {
	if b.since.IsZero() || now.Sub(b.since) >= interval {
		b.since, b.spent = now, 0
	}
	if b.spent > 0 && b.spent+rows > forcedRenderRowsPerInterval {
		return false
	}
	b.spent += rows
	return true
}
