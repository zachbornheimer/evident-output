package live

import (
	"sync/atomic"
	"time"

	"github.com/zachbornheimer/evident-output/internal/project"
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

// frameMeter remembers what the last live frame cost in wall time, from
// snapshot through write. Workers and the animator both report to it.
type frameMeter struct{ last atomic.Int64 }

func (m *frameMeter) observe(d time.Duration) { m.last.Store(int64(d)) }

func (m *frameMeter) cost() time.Duration { return time.Duration(m.last.Load()) }

const (
	// frameCostBudget is the most a frame may cost before the animator
	// treats the surface as overloaded: a quarter of a spinner period.
	frameCostBudget = project.SpinnerPeriod / 4

	// staleCeiling is the longest a Running row's visible frame may stay
	// unchanged (§14), as project.SpinnerPeriod's doc states it.
	staleCeiling = 100 * time.Millisecond

	// glyphSkipMargin is the slack kept under staleCeiling for timer and
	// scheduling jitter.
	glyphSkipMargin = 20 * time.Millisecond

	// spinnerSlotSettle is how far past a glyph-slot boundary the animator
	// wakes, so the paint reads the new slot despite wall-clock slew.
	spinnerSlotSettle = time.Millisecond
)

// skipGlyphRepaint reports whether the animator may skip this tick's
// glyph-only repaint: the last frame cost more than frameCostBudget and the
// screen was written so recently that the next tick still lands inside
// staleCeiling. After a skip sinceWrite is at least one period, so it never
// skips twice in a row and cannot break the 100ms rule.
func skipGlyphRepaint(lastCost, sinceWrite time.Duration) bool {
	if lastCost <= frameCostBudget {
		return false
	}
	return sinceWrite+project.SpinnerPeriod+spinnerSlotSettle+glyphSkipMargin < staleCeiling
}

// untilNextSpinnerSlot is the wait from now to just past the next boundary
// of project.SpinnerGlyph's slots. A free-running ticker has an arbitrary
// phase against those slots: two ticks could read the same glyph, the
// identical frame would be skipped, and a quiet Running row would sit stale
// for two periods. Waking once per slot makes every wake a changed frame.
func untilNextSpinnerSlot(now time.Time) time.Duration {
	period := int64(project.SpinnerPeriod)
	return time.Duration(period-now.UnixNano()%period) + spinnerSlotSettle
}
