package terminal

import (
	"os"

	"golang.org/x/term"
)

// Size reports the terminal dimensions for f when f is a TTY.
// ok is false when size cannot be determined (pipe, file, error).
func Size(f *os.File) (width, height int, ok bool) {
	if f == nil {
		return 0, 0, false
	}
	w, h, err := term.GetSize(int(f.Fd()))
	if err != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}
