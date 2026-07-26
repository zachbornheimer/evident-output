// Package terminal provides production terminal drivers for Evident Output.
package terminal

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// CSI / DEC private mode sequences used by the inline live-region driver.
const (
	seqHideCursor = "\x1b[?25l"
	seqShowCursor = "\x1b[?25h"
	seqEraseLine  = "\x1b[2K"
	seqCR         = "\r"
)

// ANSI is an exclusive-owner live-region driver writing ANSI to an io.Writer.
// It implements evo.LiveSurface / evo.TerminalDriver without importing evo
// (duck-typed method set).
type ANSI struct {
	mu sync.Mutex

	w            io.Writer
	width        int
	height       int
	interactive  bool
	liveLines    int
	cursorHidden bool
	id           string
}

// Option configures ANSI.
type Option func(*ANSI)

// WithSize sets width/height in cells.
func WithSize(width, height int) Option {
	return func(a *ANSI) {
		if width > 0 {
			a.width = width
		}
		if height > 0 {
			a.height = height
		}
	}
}

// WithInteractive enables cursor/live-region control sequences.
func WithInteractive(on bool) Option {
	return func(a *ANSI) { a.interactive = on }
}

// NewANSI builds a driver writing to w (typically os.Stderr).
func NewANSI(w io.Writer, opts ...Option) *ANSI {
	a := &ANSI{
		w:           w,
		width:       80,
		height:      24,
		interactive: true,
		id:          "ansi",
	}
	for _, o := range opts {
		o(a)
	}
	return a
}

// ID implements TerminalDriver.
func (a *ANSI) ID() string { return a.id }

// Columns implements LiveSurface.
func (a *ANSI) Columns() int { return a.width }

// Rows implements LiveSurface.
func (a *ANSI) Rows() int { return a.height }

// IsInteractive implements LiveSurface.
func (a *ANSI) IsInteractive() bool { return a.interactive }

// LiveLines returns the height of the current live region (for tests).
func (a *ANSI) LiveLines() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.liveLines
}

// WriteLive redraws the complete live region (full-frame, no partial diff).
func (a *ANSI) WriteLive(text string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	lines := splitLines(text)
	if a.interactive {
		a.hideCursorLocked()
		a.eraseLiveLocked()
		a.writeFrameLocked(lines)
		a.liveLines = len(lines)
		return
	}
	// Non-interactive: treat as plain multi-line write without cursor control.
	_, _ = io.WriteString(a.w, text)
	if !strings.HasSuffix(text, "\n") {
		_, _ = io.WriteString(a.w, "\n")
	}
	a.liveLines = 0
}

// ClearLive erases the previous live region and shows the cursor.
func (a *ANSI) ClearLive() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.interactive {
		a.eraseLiveLocked()
		a.showCursorLocked()
	}
	a.liveLines = 0
}

// WriteDurable appends a durable line above future live frames.
// Caller is responsible for ClearLive before durable when live is active;
// evo's debug path does clear → durable → redraw.
func (a *ANSI) WriteDurable(line string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.interactive && a.liveLines > 0 {
		// Should not happen if evo clears first; erase defensively.
		a.eraseLiveLocked()
		a.liveLines = 0
	}
	_, _ = io.WriteString(a.w, line)
	if !strings.HasSuffix(line, "\n") {
		_, _ = io.WriteString(a.w, "\n")
	}
}

// WriteFinal writes the final static projection and restores the cursor.
func (a *ANSI) WriteFinal(text string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.interactive && a.liveLines > 0 {
		a.eraseLiveLocked()
		a.liveLines = 0
	}
	if a.interactive {
		a.showCursorLocked()
	}
	_, _ = io.WriteString(a.w, text)
	if text != "" && !strings.HasSuffix(text, "\n") {
		_, _ = io.WriteString(a.w, "\n")
	}
}

func (a *ANSI) hideCursorLocked() {
	if !a.cursorHidden {
		_, _ = io.WriteString(a.w, seqHideCursor)
		a.cursorHidden = true
	}
}

func (a *ANSI) showCursorLocked() {
	if a.cursorHidden {
		_, _ = io.WriteString(a.w, seqShowCursor)
		a.cursorHidden = false
	}
}

func (a *ANSI) eraseLiveLocked() {
	if a.liveLines <= 0 {
		return
	}
	// Move to start of live region: cursor is after last line; go up (n-1),
	// then erase each line downward.
	n := a.liveLines
	if n > 1 {
		_, _ = fmt.Fprintf(a.w, "\x1b[%dA", n-1)
	}
	for i := 0; i < n; i++ {
		_, _ = io.WriteString(a.w, seqCR+seqEraseLine)
		if i < n-1 {
			_, _ = io.WriteString(a.w, "\n")
		}
	}
	// Return to origin of erased block.
	if n > 1 {
		_, _ = fmt.Fprintf(a.w, "\x1b[%dA", n-1)
	}
	_, _ = io.WriteString(a.w, seqCR)
}

func (a *ANSI) writeFrameLocked(lines []string) {
	for i, line := range lines {
		_, _ = io.WriteString(a.w, seqCR+seqEraseLine+line)
		if i < len(lines)-1 {
			_, _ = io.WriteString(a.w, "\n")
		}
	}
	if len(lines) > 0 {
		_, _ = io.WriteString(a.w, "\n")
	}
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}
