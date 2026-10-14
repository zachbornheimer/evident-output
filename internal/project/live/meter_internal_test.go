package live

import (
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/project"
)

const (
	skipProbeStep = time.Millisecond
	skipProbeSpan = 250 * time.Millisecond
)

// TestSkipGlyphRepaintStaysInsideStaleCeiling proves the animator can only
// skip a tick when the next one still lands inside the 100ms rule, for
// every cost around the budget and every time since the last write.
func TestSkipGlyphRepaintStaysInsideStaleCeiling(t *testing.T) {
	costs := []time.Duration{0, frameCostBudget, frameCostBudget + time.Nanosecond, 2 * project.SpinnerPeriod}
	for _, cost := range costs {
		for since := time.Duration(0); since <= skipProbeSpan; since += skipProbeStep {
			skipped := skipGlyphRepaint(cost, since)
			if cost <= frameCostBudget && skipped {
				t.Fatalf("cost %s within budget skipped at sinceWrite %s", cost, since)
			}
			if !skipped {
				continue
			}
			if next := since + project.SpinnerPeriod + spinnerSlotSettle + glyphSkipMargin; next >= staleCeiling {
				t.Fatalf("cost %s skipped at sinceWrite %s: next write lands at %s, past %s", cost, since, next, staleCeiling)
			}
			if since >= project.SpinnerPeriod {
				t.Fatalf("cost %s skipped at sinceWrite %s: a second consecutive skip", cost, since)
			}
		}
	}
}

func TestSkipGlyphRepaintEngagesForAnOverloadedSurface(t *testing.T) {
	if !skipGlyphRepaint(2*project.SpinnerPeriod, 0) {
		t.Fatal("an overloaded surface written just now was not skipped")
	}
}

func TestFrameMeterReportsTheLastFrame(t *testing.T) {
	var m frameMeter
	if got := m.cost(); got != 0 {
		t.Fatalf("fresh meter cost = %s, want 0", got)
	}
	m.observe(7 * time.Millisecond)
	m.observe(3 * time.Millisecond)
	if got := m.cost(); got != 3*time.Millisecond {
		t.Fatalf("cost = %s, want the last observation 3ms", got)
	}
}

func TestShouldSkipGlyphNeverSkipsAnUnwrittenSurface(t *testing.T) {
	var l Engine
	l.meter.observe(time.Second)
	if l.shouldSkipGlyph() {
		t.Fatal("skipped a glyph repaint before anything was ever written")
	}
}

func TestUntilNextSpinnerSlot(t *testing.T) {
	period := int64(project.SpinnerPeriod)
	base := time.Unix(0, 1000*period)
	cases := map[string]time.Time{
		"on boundary": base,
		"just after":  base.Add(time.Nanosecond),
		"just before": base.Add(project.SpinnerPeriod - time.Nanosecond),
		"mid slot":    base.Add(project.SpinnerPeriod / 2),
	}
	for name, now := range cases {
		t.Run(name, func(t *testing.T) {
			wait := untilNextSpinnerSlot(now)
			if wait <= 0 || wait > project.SpinnerPeriod+spinnerSlotSettle {
				t.Fatalf("wait %s outside (0, %s]", wait, project.SpinnerPeriod+spinnerSlotSettle)
			}
			current := now.UnixNano() / period
			if got := now.Add(wait).UnixNano() / period; got != current+1 {
				t.Fatalf("wake lands in slot %d, want %d", got, current+1)
			}
		})
	}
}
