package evo_test

import (
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// flakyLiveSurface is a LiveSurface whose IsInteractive() can be flipped mid-test
// (simulating terminal.ANSI disabling interactivity after a short write, see
// TestTERM007_ShortWriteDisablesInteractive) while still backing the SAME
// output stream a real terminal capture would show (construct.go wires the
// live driver's writer and Config.primary to the same underlying stream, so
// once IsInteractive() goes false, evo's plain-mode durable write lands on
// the very same stream the still-onscreen live frame was drawn to).
type flakyLiveSurface struct {
	interactive bool
	ops         []string
}

func (s *flakyLiveSurface) ID() string          { return "flaky" }
func (s *flakyLiveSurface) Columns() int        { return 80 }
func (s *flakyLiveSurface) Rows() int           { return 24 }
func (s *flakyLiveSurface) IsInteractive() bool { return s.interactive }
func (s *flakyLiveSurface) WriteLive(text string) {
	s.ops = append(s.ops, "live:"+text)
}
func (s *flakyLiveSurface) ClearLive() {
	s.ops = append(s.ops, "clear")
}
func (s *flakyLiveSurface) WriteDurable(line string) {
	s.ops = append(s.ops, "durable:"+line)
}
func (s *flakyLiveSurface) WriteFinal(text string) {
	s.ops = append(s.ops, "final:"+text)
}

// Write implements io.Writer so this same surface can also stand in for
// Config.primary — matching construct.go's real wiring where the live writer
// and the plain-mode primary writer are the same *os.File.
func (s *flakyLiveSurface) Write(p []byte) (int, error) {
	s.ops = append(s.ops, "plainwrite:"+string(p))
	return len(p), nil
}

// TestArmedLine_ClearedBeforeDurableWriteAfterInteractivityLost is the
// red-first case: the armed (title-only) live line painted by arm() before
// any entity exists must still be cleared before a durable Println lands —
// even after the live surface stops reporting itself interactive (the
// terminal.ANSI short-write path disables IsInteractive() permanently once a
// single write comes back short, per TestTERM007_ShortWriteDisablesInteractive,
// even though the underlying fd keeps accepting writes). Today
// writeDurableTextLocked's interactive branch is gated on a fresh
// live.IsInteractive() read, so once that flips false mid-run the still-armed
// spinner line is never cleared and the durable text lands glued to it.
func TestArmedLine_ClearedBeforeDurableWriteAfterInteractivityLost(t *testing.T) {
	surface := &flakyLiveSurface{interactive: true}
	out := evo.Init(evo.Config{
		Title:           "zq",
		Terminal:        surface,
		Stdout:          surface,
		VisibilityDelay: evo.Delay(0),
	})
	t.Cleanup(func() { _ = out.Close() })

	// arm()'s first-paint: title-only live line before any Task/Item exists.
	if len(surface.ops) == 0 || !strings.HasPrefix(surface.ops[0], "live:") {
		t.Fatalf("setup: expected an armed live frame first, got %+v", surface.ops)
	}

	// Simulate terminal.ANSI's short-write path disabling interactivity while
	// the armed spinner line is still on screen.
	surface.interactive = false

	out.Println("zq setup: resolving repository and tools…")

	// Find the last live-region write before the durable write we just made,
	// and confirm a clear happened between them.
	durableIdx := -1
	for i, op := range surface.ops {
		if strings.HasPrefix(op, "durable:") || strings.HasPrefix(op, "plainwrite:") {
			durableIdx = i
		}
	}
	if durableIdx == -1 {
		t.Fatalf("expected a durable/plain write recorded, got %+v", surface.ops)
	}
	sawClear := false
	for i := 0; i < durableIdx; i++ {
		if surface.ops[i] == "clear" {
			sawClear = true
		}
	}
	if !sawClear {
		t.Fatalf("durable write at %d has no preceding clear of the armed line: %+v", durableIdx, surface.ops)
	}
}
