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
// On a TTY: color allowed (callers that honor NO_COLOR should pass NoColor
// themselves when the env is set). Off-TTY (*os.File that is not a char device):
// Plain + NoColor so piped/agent logs stay free of CSI. Non-file writers (buffers)
// are left unchanged so tests can still assert color rendering.
//
// Always includes To(w). Pass Diagnostics(os.Stderr) in extra for dual-stream
// CLIs — Debug and Capture mirrors go to Diagnostics; Items/Tasks stay on w.
// Extra options are applied after defaults (they win on conflicts when options
// overwrite the same field).
//
//	out := evo.For("tool", evo.WriterOptions(os.Stdout, evo.Diagnostics(os.Stderr))...)
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
