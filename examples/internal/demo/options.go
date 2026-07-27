// Package demo holds shared helpers for example CLIs.
package demo

import (
	"io"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

// Options returns presentation options for example programs.
// Color is on by default; honor NO_COLOR like real CLIs should.
func Options(human io.Writer, extra ...evo.Option) []evo.Option {
	opts := []evo.Option{
		evo.To(human),
		evo.Plain(), // final report (not live region); color still enabled
	}
	if os.Getenv("NO_COLOR") != "" {
		opts = append(opts, evo.NoColor())
	}
	opts = append(opts, extra...)
	return opts
}
