package terminal_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/terminal"
)

// shortWriteAfterN is an io.Writer that fully records every write (so the
// test can inspect exactly what a real pty would have received) but reports
// a short write (n < len(p), no error — matching a real partial pty write
// under buffer pressure) starting from the Nth call onward. That partial
// report is enough to trip terminal.ANSI's TERM-007 handling and disable
// IsInteractive() permanently, even though every byte given to it did land.
type shortWriteAfterN struct {
	buf   bytes.Buffer
	calls int
	after int
}

func (w *shortWriteAfterN) Write(p []byte) (int, error) {
	w.calls++
	w.buf.Write(p)
	if w.calls >= w.after {
		return len(p) - 1, nil
	}
	return len(p), nil
}

// TestANSI_ClearLiveErasesStaleFrameAfterShortWriteDisablesInteractive is the
// red-first case: a short write on the very frame that painted the armed
// (or any) live line disables IsInteractive() (TestTERM007) while that frame
// is still, in reality, sitting on the terminal — the short write only
// truncated the driver's *count*, the pty still received every byte. A
// caller (evo's writeDurableTextLocked) that dutifully calls ClearLive()
// before writing a durable line must still get real erase escapes: gating
// the erase on the *current* interactive flag instead of on liveLines (an
// on-screen frame was actually drawn) leaves the stale frame permanently
// unerased, and every following durable write lands glued to it.
func TestANSI_ClearLiveErasesStaleFrameAfterShortWriteDisablesInteractive(t *testing.T) {
	// hideCursorLocked + writeFrameLocked issue exactly two writes for the
	// first live frame (eraseLiveLocked no-ops when liveLines==0); make the
	// second of those short so interactivity disables mid-paint while the
	// frame itself still landed in full.
	w := &shortWriteAfterN{after: 2}
	drv := terminal.NewANSI(w, terminal.WithInteractive(true), terminal.WithSize(80, 24))

	drv.WriteLive("⠋  zq")
	if drv.IsInteractive() {
		t.Fatal("setup: expected the short write to disable interactivity")
	}
	if drv.LiveLines() == 0 {
		t.Fatal("setup: expected the frame to still be tracked as on-screen")
	}

	drv.ClearLive()
	drv.WriteDurable("zq setup: resolving repository and tools…")

	got := w.buf.String()
	frameIdx := strings.Index(got, "⠋  zq")
	durableIdx := strings.Index(got, "zq setup:")
	if frameIdx < 0 || durableIdx < 0 {
		t.Fatalf("expected both frame and durable text in output: %q", got)
	}
	between := got[frameIdx+len("⠋  zq") : durableIdx]
	if !strings.Contains(between, "\x1b[2K") {
		t.Fatalf("durable write after a short-write-disabled ClearLive was not preceded by an erase sequence: %q (between=%q)", got, between)
	}
}
