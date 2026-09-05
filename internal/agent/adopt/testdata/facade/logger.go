// Fixture for adopt's custom-output-facade detection. This mirrors the
// shape go-task's internal/logger takes: a struct holding io.Writer fields,
// with methods that route every real call through a color-printer closure
// or fmt.Fprint* rather than a bare fmt/os call adopt already recognizes.
// Do not "fix" these call sites; the facade tests pin their exact shape.
package facade

import (
	"fmt"
	"io"

	"github.com/fatih/color"
)

// Logger is the wrapped-output facade: every status line in this package
// goes through Outf/Errf, never through a bare fmt.Print or os.Stdout write
// adopt's per-call classifier would catch on its own.
type Logger struct {
	Stdout io.Writer
	Stderr io.Writer
}

// Outf writes a normal-color status line to Stdout via a color-printer
// closure — the pattern adopt's per-call classifier cannot see through.
func (l *Logger) Outf(format string, args ...interface{}) {
	color.New(color.Reset).FprintfFunc()(l.Stdout, format, args...)
}

// Errf writes a red status line to Stderr the same way.
func (l *Logger) Errf(format string, args ...interface{}) {
	color.New(color.FgRed).FprintfFunc()(l.Stderr, format, args...)
}

// Warnf falls back to a plain fmt.Fprintf against the wrapped Stderr field
// — still facade output, just without the color-printer indirection.
func (l *Logger) Warnf(format string, args ...interface{}) {
	fmt.Fprintf(l.Stderr, format, args...)
}
