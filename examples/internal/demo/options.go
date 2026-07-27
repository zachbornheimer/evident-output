// Package demo holds shared helpers for example CLIs.
package demo

import (
	"io"
	"os"
	"strings"

	evo "github.com/zachbornheimer/evident-output"
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

// Options returns presentation options for example programs.
func Options(human io.Writer, color ColorMode, extra ...evo.Option) []evo.Option {
	opts := []evo.Option{
		evo.To(human),
		evo.Plain(), // final report (not live region); color still allowed
	}
	switch color {
	case ColorNever:
		opts = append(opts, evo.NoColor())
	case ColorAlways:
		// emit color even when NO_COLOR is set (explicit demo request)
	case ColorAuto:
		if os.Getenv("NO_COLOR") != "" {
			opts = append(opts, evo.NoColor())
		}
	}
	opts = append(opts, extra...)
	return opts
}
