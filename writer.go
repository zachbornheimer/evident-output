package evo

import (
	"io"
	"os"
)

// IsCharDevice reports whether w is an *os.File backed by a character device
// (typical interactive TTY). Pipes, files, and non-file writers return false.
//
// Use when choosing Plain / NoColor defaults so agents capturing CLI output
// do not get ANSI noise.
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

// WriterOptions returns presentation options appropriate for human writer w.
//
// On a TTY: color allowed (unless NO_COLOR is set by the process environment —
// callers that want env policy should pass NoColor themselves or use examples/demo).
// Off-TTY (pipe, file, bytes.Buffer is NOT auto-detected here): when w is an
// *os.File that is not a char device, returns Plain + NoColor so piped logs stay
// free of CSI. Non-file writers (buffers, multi-writers) are left unchanged so
// tests can still assert color rendering.
//
// Always includes To(w). Extra options are appended and win over defaults when
// they conflict (last Option applied in New/For order — pass extra after).
func WriterOptions(w io.Writer, extra ...Option) []Option {
	opts := []Option{To(w)}
	if f, ok := w.(*os.File); ok {
		if !IsCharDevice(f) {
			opts = append(opts, Plain(), NoColor())
		}
	}
	opts = append(opts, extra...)
	return opts
}
