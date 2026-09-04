package evo

import (
	"io"
	"os"
)

// IsCharDevice reports whether w is an *os.File backed by a character device
// (typical interactive TTY). Pipes, files, and non-file writers return false.
//
// Prefer Config{Stdout, Stderr} for ordinary dual-stream construction. This
// helper remains for hosts that choose Plain/NoColor from a concrete writer.
func IsCharDevice(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

// sameTerminalDevice reports whether a and b are both *os.File values backed
// by the same physical character device — the case where two distinct
// io.Writer values (e.g. Config's Stdout and Stderr) nonetheless name one
// controlling terminal, because a shell attached both fds to the same tty
// without redirection. os.SameFile compares the underlying device/inode, so
// it answers true here even though a and b are different *os.File objects.
//
// construct.go uses this at construction time (configToOptions) to detect
// when Diagnostics shares the live region's terminal — never inferred later
// from writer identity alone, which only catches the same-object case.
func sameTerminalDevice(a, b io.Writer) bool {
	fa, ok := a.(*os.File)
	if !ok {
		return false
	}
	fb, ok := b.(*os.File)
	if !ok {
		return false
	}
	sa, err := fa.Stat()
	if err != nil || sa.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	sb, err := fb.Stat()
	if err != nil || sb.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	return os.SameFile(sa, sb)
}
