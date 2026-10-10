package terminal

import (
	"io"
	"os"
	"sync"
)

// Driver is the exclusive owner of terminal control sequences. ANSI and
// testkit's recording screen implement it.
type Driver interface {
	ID() string
}

// charDeviceOverride is the TTY-detection test seam. Tests mark one concrete
// writer as a character device so a bytes.Buffer can stand in for a pty
// without claiming every writer in the process is a TTY.
var charDeviceOverride struct {
	mu     sync.RWMutex
	writer io.Writer
	on     bool
}

// WriterIsCharDevice reports whether w is a terminal for construction
// purposes: a writer marked by MarkCharDevice, or an *os.File that IsCharDevice.
func WriterIsCharDevice(w io.Writer) bool {
	charDeviceOverride.mu.RLock()
	marked, on := charDeviceOverride.writer, charDeviceOverride.on
	charDeviceOverride.mu.RUnlock()
	if on && w != nil && w == marked {
		return true
	}
	return IsCharDevice(w)
}

// MarkCharDevice treats w as a TTY during construction so tests can exercise
// live-region and color inference without opening a pty. Only this writer
// matches; other writers still use the OS check. The returned func restores
// the previous mark.
func MarkCharDevice(w io.Writer) func() {
	charDeviceOverride.mu.Lock()
	prevW, prevOn := charDeviceOverride.writer, charDeviceOverride.on
	charDeviceOverride.writer = w
	charDeviceOverride.on = true
	charDeviceOverride.mu.Unlock()
	return func() {
		charDeviceOverride.mu.Lock()
		charDeviceOverride.writer = prevW
		charDeviceOverride.on = prevOn
		charDeviceOverride.mu.Unlock()
	}
}

// IsCharDevice reports whether w is an *os.File backed by a character device
// (typical interactive TTY). Pipes, files, and non-file writers return false.
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

// SameDevice reports whether a and b are both *os.File values backed by the
// same physical character device — the case where two distinct io.Writer
// values (Config's Stdout and Stderr) nonetheless name one controlling
// terminal, because a shell attached both fds to the same tty without
// redirection. os.SameFile compares the underlying device and inode, so it
// answers true even though a and b are different *os.File objects.
func SameDevice(a, b io.Writer) bool {
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
