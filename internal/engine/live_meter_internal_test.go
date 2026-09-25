package engine

import (
	"testing"
	"time"
)

// TestRenderCostCoolsDownInProportionToCost pins the E-030 meter: a paint
// that cost d holds off the next one for d*(renderDutyFactor-1), and a
// paint cheaper than renderCostFloor holds off nothing.
func TestRenderCostCoolsDownInProportionToCost(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const cost = 100 * time.Millisecond
	var c renderCost
	for range renderCostSamples {
		c.record(t0, t0.Add(cost))
	}
	end := t0.Add(cost)
	repaid := end.Add(cost * (renderDutyFactor - 1))
	if !c.coolingDown(end) {
		t.Errorf("coolingDown right after a %s paint = false; want true", cost)
	}
	if !c.coolingDown(repaid.Add(-time.Millisecond)) {
		t.Errorf("coolingDown just before the paint is repaid = false; want true")
	}
	if c.coolingDown(repaid) {
		t.Errorf("coolingDown once the paint is repaid = true; want false")
	}
	var cheap renderCost
	cheap.record(t0, t0.Add(renderCostFloor-time.Microsecond))
	if cheap.coolingDown(t0) {
		t.Errorf("coolingDown after a paint under renderCostFloor = true; want false")
	}
}

// TestRenderCostIgnoresOneSlowFrame proves a single slow frame — a GC
// pause or preemption during a small render — does not hold off the next
// paint; only frames that are slow every time do.
func TestRenderCostIgnoresOneSlowFrame(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var c renderCost
	c.record(t0, t0.Add(time.Millisecond))
	c.record(t0, t0.Add(time.Millisecond))
	c.record(t0, t0.Add(50*time.Millisecond))
	if c.coolingDown(t0.Add(50 * time.Millisecond)) {
		t.Errorf("coolingDown after one slow frame among cheap ones = true; want false")
	}
}
