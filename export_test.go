package evo

import (
	"io"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// SwapLookupEnv replaces the process-environment facade. Tests inject a
// map instead of racing os.Setenv. Restore via the returned function.
func SwapLookupEnv(fn func(string) string) func() {
	return engine.SwapLookupEnv(fn)
}

// MarkWriterAsCharDevice treats w as a TTY during construction so tests can
// exercise live-region / color inference without opening a pty. Only this
// writer matches; other tests' writers still use the OS check.
func MarkWriterAsCharDevice(w io.Writer) func() {
	return engine.MarkWriterAsCharDevice(w)
}
