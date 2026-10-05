package evo_test

import (
	"io"
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
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Clock: clock, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })

	g := out.Group("work")
	indeterminate := g.Task("verify")
	bar := g.Task("scan")

	// First paint.
	indeterminate.Doing("checking")
	first := screen.LatestLiveText()
	// Advance clock via progress on sibling — each Progress calls clock.Now() for render.
	bar.Progress(1, 10)
	second := screen.LatestLiveText()
	bar.Progress(2, 10)
	third := screen.LatestLiveText()

	// Extract first rune of the verify line spinner (child line containing "verify").
	glyph := func(live string) string {
		for line := range strings.SplitSeq(live, "\n") {
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

// TestLive_GroupTwoSpinnerFrame proves DisplayGroup's concurrency
// truth: two children both Running render the SAME shared spinner frame at
// once — a plain collection documents its children as independent, so
// concurrent Running rows are the expected shape, not a defect a "one
// Running child" heart contract would forbid (that contract belongs to
// Sequence alone).
func TestLive_GroupTwoSpinnerFrame(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Clock: clock, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })

	jobs := out.Group("dependencies")
	a := jobs.Task("discover")
	b := jobs.Task("verify")

	a.Doing("discovering")
	b.Doing("verifying") // second Running sibling — no misuse, unlike Sequence

	if err := out.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil (DisplayGroup permits concurrent Running)", err)
	}

	frame := screen.LatestLiveText()
	lines := strings.Split(frame, "\n")
	var spinnerLines []string
	for _, line := range lines {
		if strings.Contains(line, "discover") || strings.Contains(line, "verify") {
			spinnerLines = append(spinnerLines, line)
		}
	}
	if len(spinnerLines) != 2 {
		t.Fatalf("want 2 Running child rows, got %d:\n%s", len(spinnerLines), frame)
	}
	// Both rows carry the same leading glyph column — the shared spinner
	// frame both Running children get at the same instant.
	glyphOf := func(line string) string {
		trimmed := strings.TrimLeft(line, " ")
		// The glyph-to-name gap is a single space (DisplayUnit.Render); the
		// name-to-detail gap is a double space, so a single-space split
		// isolates the leading glyph regardless of name padding width.
		fields := strings.SplitN(trimmed, " ", 2)
		return fields[0]
	}
	if glyphOf(spinnerLines[0]) != glyphOf(spinnerLines[1]) {
		t.Fatalf("two concurrent Running children must share one spinner frame:\n%s", frame)
	}

	succeed(a)
	succeed(b)
	_ = out.Finish()
}
