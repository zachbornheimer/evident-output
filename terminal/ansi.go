// Package terminal provides production terminal drivers for Evident Output.
package terminal

import (
	"fmt"
	"io"
	"os"
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
	writeErr     error // first write failure (TERM-007)

	// sizeFile, when set, is re-queried on RefreshSize (resize / SIGWINCH path).
	sizeFile *os.File
	// resize holds the optional SIGWINCH subscription (unix only).
	resize *resizeWatch
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

// WithSizeFile enables RefreshSize to re-query geometry from a TTY file
// (typically the same *os.File used as the live writer). Call RefreshSize on
// each live redraw. On unix, evo also starts StartResizeWatch so SIGWINCH
// updates size and can force an immediate live redraw.
func WithSizeFile(f *os.File) Option {
	return func(a *ANSI) {
		if f != nil {
			a.sizeFile = f
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
func (a *ANSI) Columns() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.width
}

// Rows implements LiveSurface.
func (a *ANSI) Rows() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.height
}

// SetSize updates stored geometry (tests and external SIGWINCH handlers).
func (a *ANSI) SetSize(width, height int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if width > 0 {
		a.width = width
	}
	if height > 0 {
		a.height = height
	}
}

// RefreshSize re-queries the TTY when WithSizeFile was configured.
// Safe to call on every redraw; no-op when sizeFile is unset.
func (a *ANSI) RefreshSize() {
	a.mu.Lock()
	f := a.sizeFile
	a.mu.Unlock()
	if f == nil {
		return
	}
	w, h, ok := Size(f)
	if !ok {
		return
	}
	a.SetSize(w, h)
}

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
	a.writeStringLocked(text)
	if !strings.HasSuffix(text, "\n") {
		a.writeStringLocked("\n")
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
	a.writeStringLocked(line)
	if !strings.HasSuffix(line, "\n") {
		a.writeStringLocked("\n")
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
	a.writeStringLocked(text)
	if text != "" && !strings.HasSuffix(text, "\n") {
		a.writeStringLocked("\n")
	}
}

// WriteErr returns the first write failure observed (TERM-007 short-write path).
func (a *ANSI) WriteErr() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.writeErr
}

func (a *ANSI) noteWriteErrLocked(err error) {
	if err == nil {
		return
	}
	if a.writeErr == nil {
		a.writeErr = err
	}
	// Disable interactivity safely so further frames skip cursor control.
	if a.interactive {
		a.interactive = false
		a.cursorHidden = false
	}
}

func (a *ANSI) writeStringLocked(s string) {
	n, err := io.WriteString(a.w, s)
	if err != nil {
		a.noteWriteErrLocked(err)
		return
	}
	if n < len(s) {
		a.noteWriteErrLocked(io.ErrShortWrite)
	}
}

func (a *ANSI) hideCursorLocked() {
	if !a.cursorHidden {
		a.writeStringLocked(seqHideCursor)
		if a.writeErr == nil {
			a.cursorHidden = true
		}
	}
}

func (a *ANSI) showCursorLocked() {
	if a.cursorHidden {
		a.writeStringLocked(seqShowCursor)
		if a.writeErr == nil {
			a.cursorHidden = false
		}
	}
}

func (a *ANSI) eraseLiveLocked() {
	if a.liveLines <= 0 {
		return
	}
	// Cursor rests on the last line of the live region (writeFrame does not
	// leave a trailing blank line). Move to first line with CUU (n-1), erase
	// each line downward, then return to origin.
	n := a.liveLines
	if n > 1 {
		a.writeStringLocked(fmt.Sprintf("\x1b[%dA", n-1))
	}
	for i := 0; i < n; i++ {
		a.writeStringLocked(seqCR + seqEraseLine)
		if i < n-1 {
			a.writeStringLocked("\n")
		}
	}
	if n > 1 {
		a.writeStringLocked(fmt.Sprintf("\x1b[%dA", n-1))
	}
	a.writeStringLocked(seqCR)
}

func (a *ANSI) writeFrameLocked(lines []string) {
	// Write lines without a trailing blank newline so the cursor stays on the
	// last live line — required for eraseLiveLocked's CUU arithmetic.
	for i, line := range lines {
		a.writeStringLocked(seqCR + seqEraseLine + line)
		if i < len(lines)-1 {
			a.writeStringLocked("\n")
		}
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
