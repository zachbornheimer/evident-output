package terminal

import (
	"io"
	"os"
)

// Stdin returns the process's standard input.
func Stdin() io.Reader { return os.Stdin }

// Stdout returns the process's standard output.
func Stdout() io.Writer { return os.Stdout }

// Stderr returns the process's standard error.
func Stderr() io.Writer { return os.Stderr }

// WriterSize reports the terminal dimensions of w when w is a TTY file.
// ok is false when w is not a file or its size cannot be determined.
func WriterSize(w io.Writer) (width, height int, ok bool) {
	f, isFile := w.(*os.File)
	if !isFile {
		return 0, 0, false
	}
	return Size(f)
}

// WithSizeWriter re-queries w's terminal geometry on each live redraw when w
// is a file; for any other writer it changes nothing.
func WithSizeWriter(w io.Writer) Option {
	return func(a *ANSI) {
		if f, isFile := w.(*os.File); isFile {
			WithSizeFile(f)(a)
		}
	}
}
