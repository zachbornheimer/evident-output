package engine

import (
	"slices"
	"time"
)

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

// renderCost meters the wall time live frame builds hold o.mu. A frame
// renders the whole live region under the lock, so its cost grows with
// the run: at 16000 Tasks one frame took about 143ms, longer than the 50ms
// frame interval, and paints back to back starved the scheduler (9m36s
// for no-op Tasks). After frames that cost d, the next one waits until
// d*(renderDutyFactor-1) has passed, so painting holds the lock for at
// most 1/renderDutyFactor of the run however large it grows.
//
// d is the cheapest of the last renderCostSamples frames, so one GC pause
// or preemption during a small frame cannot hold off the next paint;
// frames that are genuinely large are large every time.
type renderCost struct {
	recent [renderCostSamples]time.Duration
	next   int
	ended  time.Time
}

// renderCostSamples is how many recent frame costs the cooldown reads.
const renderCostSamples = 3

// renderDutyFactor is the inverse of the largest share of wall time live
// paints may hold o.mu once frames cost at least renderCostFloor.
const renderDutyFactor = 4

// renderCostFloor is the frame cost below which no cooldown applies, so
// runs up to a few thousand rows keep painting every transition the frame
// rate allows (FP-005: spinner before check). Under it, the frame-rate
// cap and the forced-paint budget already bound the lock time; the
// cooldown is for frames too large for either to hold.
const renderCostFloor = 10 * time.Millisecond

// record notes a frame build that held the lock from began to ended.
func (c *renderCost) record(began, ended time.Time) {
	c.recent[c.next] = ended.Sub(began)
	c.next = (c.next + 1) % renderCostSamples
	c.ended = ended
}

// coolingDown reports whether the recent frames have not yet been repaid
// at now.
func (c *renderCost) coolingDown(now time.Time) bool {
	cost := slices.Min(c.recent[:])
	if cost < renderCostFloor {
		return false
	}
	return now.Before(c.ended.Add(cost * (renderDutyFactor - 1)))
}
