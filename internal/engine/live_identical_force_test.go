package engine

import (
	"sync"
	"testing"
	"time"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// countingLiveSurface records WriteLive calls for identical-frame skip tests.
type countingLiveSurface struct {
	mu     sync.Mutex
	frames int
	latest string
}

func (s *countingLiveSurface) ID() string          { return "counting-live" }
func (s *countingLiveSurface) Columns() int        { return 80 }
func (s *countingLiveSurface) Rows() int           { return 24 }
func (s *countingLiveSurface) IsInteractive() bool { return true }
func (s *countingLiveSurface) ClearLive()          {}
func (s *countingLiveSurface) WriteDurable(string) {}
func (s *countingLiveSurface) WriteFinal(string)   {}
func (s *countingLiveSurface) WriteLive(text string) {
	s.mu.Lock()
	s.frames++
	s.latest = text
	s.mu.Unlock()
}
func (s *countingLiveSurface) frameCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.frames
}
func (s *countingLiveSurface) latestText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latest
}

// TestLive_ForcedIdenticalFrameSkipsWriteLive proves force=true still skips
// WriteLive when the rendered live text equals the last painted frame
// (spinner ticks pass force=true). Advancing the clock enough for a spinner
// glyph change must still paint a new frame.
func TestLive_ForcedIdenticalFrameSkipsWriteLive(t *testing.T) {
	screen := &countingLiveSurface{}
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", withTerminal(screen), visibilityDelay(0), withClock(clock), withNoColor())
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("verify")
	task.Doing("checking")

	framesAfterFirst := screen.frameCount()
	if framesAfterFirst < 1 {
		t.Fatal("expected at least one live frame after Doing")
	}
	firstText := screen.latestText()

	out.mu.Lock()
	out.renderLiveLocked(true)
	out.mu.Unlock()

	if got := screen.frameCount(); got != framesAfterFirst {
		t.Fatalf("forced redraw with identical text re-painted: LiveFrameCount %d → %d", framesAfterFirst, got)
	}
	if got := screen.latestText(); got != firstText {
		t.Fatalf("latest live text changed without a new paint:\nwas %q\ngot %q", firstText, got)
	}

	// Advance past one spinner period so the glyph in the rendered string changes.
	clock.Advance(txt.SpinnerPeriod + time.Millisecond)

	out.mu.Lock()
	out.renderLiveLocked(true)
	out.mu.Unlock()

	if got := screen.frameCount(); got <= framesAfterFirst {
		t.Fatalf("clock-advanced forced redraw did not paint: LiveFrameCount stayed %d", got)
	}
	if got := screen.latestText(); got == firstText {
		t.Fatalf("expected spinner glyph change after clock advance; live text unchanged:\n%s", got)
	}
}
