// Package plain is the non-TTY text entry point. Its own row/ledger/
// conclusion writers live in the parent render package because live (the
// TTY entry point) needs the same ones — see internal/render's row_*.go.
// This package now holds only the plain-mode composition itself, so that
// neither entry point depends on the other (render/live must never import
// render/plain, and render/plain must never import render/live).
package plain

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Plain projects a snapshot to plain text without terminal ownership.
// width <= 0 falls back to render.DefaultWidth.
func Plain(s core.Snapshot, width int, noColor, verbose bool, profile txt.GlyphProfile) string {
	return render.Plain(s, width, noColor, verbose, profile)
}
