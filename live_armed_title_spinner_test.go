package evo

import (
	"strings"
	"testing"
	"time"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// TestLive_ArmedTitleKeepsAnimatorAndAdvancesGlyph is the red-first proof that
// Init's title-only live line ("⠦  zq") must keep the spinner animator alive
// while no Task exists. needsSpinnerAnimLocked used to look only at
// Running/Pending tasks, so ensureReady-style work after arm() froze the
// header spinner on screen.
func TestLive_ArmedTitleKeepsAnimatorAndAdvancesGlyph(t *testing.T) {
	drv := &fakeHeartbeatSurface{}
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("zq", withTerminal(drv), visibilityDelay(0), withClock(clock), withNoColor(), glyphs(GlyphsUnicode))
	t.Cleanup(func() { _ = out.Close() })

	out.arm()

	first := drv.latest()
	if first == "" {
		t.Fatal("armed title must paint a live frame")
	}
	if !strings.Contains(first, "zq") {
		t.Fatalf("armed title missing subject:\n%s", first)
	}

	out.mu.Lock()
	needs := out.needsSpinnerAnimLocked()
	running := out.live != nil && out.live.animRunning
	out.mu.Unlock()
	if !needs {
		t.Fatal("armed title spinner must keep needsSpinnerAnimLocked true")
	}
	if !running {
		t.Fatal("animator must be running while the armed title is on screen")
	}

	clock.Advance(txt.SpinnerPeriod + time.Millisecond)
	out.mu.Lock()
	out.renderLiveLocked(true)
	out.mu.Unlock()
	second := drv.latest()
	if second == first {
		t.Fatalf("armed title spinner did not advance after one SpinnerPeriod:\n%s", second)
	}
}
