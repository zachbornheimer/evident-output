package evo_test

import (
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// advancingClock is a TimeSource that steps on each Now() for spinner tests.
type advancingClock struct {
	t time.Time
	d time.Duration
}

func (c *advancingClock) Now() time.Time {
	cur := c.t
	c.t = c.t.Add(c.d)
	return cur
}

func TestLive_SpinnerGlyphAdvancesWithClock(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := &advancingClock{
		t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		d: 80 * time.Millisecond, // one spinner frame per Now()
	}
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.Clock(clock), evo.VisibilityDelay(0), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	g := out.Tasks("work")
	indeterminate := g.Task("verify")
	bar := g.Task("scan")

	// First paint.
	indeterminate.Phase("checking")
	first := screen.LatestLiveText()
	// Advance clock via progress on sibling — each Progress calls clock.Now() for render.
	bar.Progress(1, 10)
	second := screen.LatestLiveText()
	bar.Progress(2, 10)
	third := screen.LatestLiveText()

	// Extract first rune of the verify line spinner (child line containing "verify").
	glyph := func(live string) string {
		for _, line := range strings.Split(live, "\n") {
			if strings.Contains(line, "verify") {
				fields := strings.Fields(line)
				if len(fields) > 0 {
					return fields[0]
				}
			}
		}
		return ""
	}
	g1, g2, g3 := glyph(first), glyph(second), glyph(third)
	if g1 == "" || g2 == "" || g3 == "" {
		t.Fatalf("missing verify lines:\n1:%q\n2:%q\n3:%q", first, second, third)
	}
	// At least one advance across three frames (period-aligned).
	if g1 == g2 && g2 == g3 {
		t.Fatalf("spinner did not advance: %q %q %q\n%s\n---\n%s", g1, g2, g3, first, third)
	}
	// scan bar still present alongside verify (independent rows).
	if !strings.Contains(third, "scan") || !strings.Contains(third, "[") {
		t.Fatalf("expected scan bar alongside verify:\n%s", third)
	}
}
