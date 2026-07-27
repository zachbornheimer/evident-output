// Package demo holds shared helpers for example CLIs.
package demo

import (
	"io"
	"os"
	"strings"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/terminal"
)

// ColorMode selects whether the example emits SGR color.
type ColorMode int

const (
	// ColorAuto: color unless NO_COLOR is set (no-color.org).
	ColorAuto ColorMode = iota
	// ColorAlways: always color (demo / force).
	ColorAlways
	// ColorNever: never color.
	ColorNever
)

// ParseColorFlag maps always|never|auto (default auto).
func ParseColorFlag(s string) ColorMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "always", "on", "yes", "true", "1":
		return ColorAlways
	case "never", "off", "no", "false", "0":
		return ColorNever
	default:
		return ColorAuto
	}
}

// IsCharDevice reports whether w is a character device (typical TTY).
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

// Options returns presentation options for example programs.
//
// On a TTY, attaches an ANSI live-region driver so Task.Phase / Progress show
// indeterminate spinners and resolved rows stream as durable evidence (spec §1).
// Off-TTY (pipes, mise batch): Plain + NoColor by default so captured logs have
// no CSI (ColorAlways still forces color when requested).
func Options(human io.Writer, color ColorMode, extra ...evo.Option) []evo.Option {
	opts := []evo.Option{
		evo.To(human),
	}
	tty := IsCharDevice(human)
	if tty {
		opts = append(opts,
			evo.Terminal(terminal.NewANSI(human,
				terminal.WithInteractive(true),
				terminal.WithSize(80, 24),
			)),
			// Demo: show spinners immediately (real apps use default ~150ms).
			evo.VisibilityDelay(0),
		)
	} else {
		// Progressive durable lines still stream; no CSI live region.
		opts = append(opts, evo.Plain())
	}
	switch color {
	case ColorNever:
		opts = append(opts, evo.NoColor())
	case ColorAlways:
		// emit color even when NO_COLOR is set or off-TTY (explicit demo request)
	case ColorAuto:
		if os.Getenv("NO_COLOR") != "" || !tty {
			opts = append(opts, evo.NoColor())
		}
	}
	opts = append(opts, extra...)
	return opts
}
