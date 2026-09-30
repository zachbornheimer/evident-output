package engine

import (
	"testing"
	"time"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

const (
	skipProbeStep = time.Millisecond
	skipProbeSpan = 250 * time.Millisecond
)

// TestSkipGlyphRepaintStaysInsideStaleCeiling proves the animator can only
// skip a tick when the next one still lands inside the 100ms rule, for
// every cost around the budget and every time since the last write.
func TestSkipGlyphRepaintStaysInsideStaleCeiling(t *testing.T) {
	costs := []time.Duration{0, frameCostBudget, frameCostBudget + time.Nanosecond, 2 * txt.SpinnerPeriod}
	for _, cost := range costs {
		for since := time.Duration(0); since <= skipProbeSpan; since += skipProbeStep {
			skipped := skipGlyphRepaint(cost, since)
			if cost <= frameCostBudget && skipped {
				t.Fatalf("cost %s within budget skipped at sinceWrite %s", cost, since)
			}
			if !skipped {
				continue
			}
			if next := since + txt.SpinnerPeriod + spinnerSlotSettle + glyphSkipMargin; next >= liveStaleCeiling {
				t.Fatalf("cost %s skipped at sinceWrite %s: next write lands at %s, past %s", cost, since, next, liveStaleCeiling)
			}
			if since >= txt.SpinnerPeriod {
				t.Fatalf("cost %s skipped at sinceWrite %s: a second consecutive skip", cost, since)
			}
		}
	}
}

func TestSkipGlyphRepaintEngagesForAnOverloadedSurface(t *testing.T) {
	if !skipGlyphRepaint(2*txt.SpinnerPeriod, 0) {
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
	var l liveEngine
	l.meter.observe(time.Second)
	if l.shouldSkipGlyph() {
		t.Fatal("skipped a glyph repaint before anything was ever written")
	}
}
