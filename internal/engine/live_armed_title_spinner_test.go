package engine

import (
	"strings"
	"testing"
	"time"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

func TestLive_ArmedTitlePaintsImmediatelyDespiteVisibilityDelay(t *testing.T) {
	drv := &fakeHeartbeatSurface{}
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("zq", Terminal(drv), VisibilityDelay(80*time.Millisecond), Clock(clock), NoColor(), Glyphs(GlyphsUnicode))
	t.Cleanup(func() { _ = out.Close() })

	out.arm()

	frame := drv.latest()
	if frame == "" {
		t.Fatal("armed title must paint immediately; VisibilityDelay must not hide Init's first spinner")
	}
	if !strings.Contains(frame, "zq") {
		t.Fatalf("armed title missing subject:\n%s", frame)
	}
	spin := false
	for _, g := range txt.SpinnerFrames(GlyphsUnicode) {
		if strings.Contains(frame, g) {
			spin = true
			break
		}
	}
	if !spin {
		t.Fatalf("armed title missing spinner glyph:\n%s", frame)
	}
}
