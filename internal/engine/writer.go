package engine

import (
	"io"

	"github.com/zachbornheimer/evident-output/internal/terminal"
)

// writerIsCharDevice is the TTY-detection facade construction uses.
func writerIsCharDevice(w io.Writer) bool { return terminal.WriterIsCharDevice(w) }

// MarkWriterAsCharDevice treats w as a TTY during construction so tests can
// exercise live-region / color inference without opening a pty. Only this
// writer matches; other tests' writers still use the OS check.
func MarkWriterAsCharDevice(w io.Writer) func() { return terminal.MarkCharDevice(w) }

// IsCharDevice reports whether w is an *os.File backed by a character device
// (typical interactive TTY). Pipes, files, and non-file writers return false.
//
// Prefer Config{Stdout, Stderr} for ordinary dual-stream construction. This
// helper remains for hosts that choose Plain/NoColor from a concrete writer.
func IsCharDevice(w io.Writer) bool { return terminal.IsCharDevice(w) }

// sameTerminalDevice reports whether a and b name one physical character
// device. construct.go uses this at construction time (configToOptions) to
// detect when Diagnostics shares the live region's terminal — never inferred
// later from writer identity alone, which only catches the same-object case.
func sameTerminalDevice(a, b io.Writer) bool { return terminal.SameDevice(a, b) }
