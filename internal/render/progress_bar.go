package render

import (
	"fmt"
	"strings"
)

// ProgressBar returns a fixed-width ASCII bar for completed/total.
func ProgressBar(completed, total int64, width int) string {
	if width < 4 {
		width = 4
	}
	if total <= 0 {
		return "[" + strings.Repeat("?", width) + "]"
	}
	filled := int(float64(width) * float64(completed) / float64(total))
	if completed > 0 && filled == 0 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	// Empty cells are literal spaces (spec §23: "never shaded/outline
	// glyphs") — a shaded "░" cell reads as its own semantic state (some
	// libraries use it for "paused"/"buffered"), which the bar does not
	// have and must not imply.
	return "[" + strings.Repeat("█", filled) + strings.Repeat(" ", width-filled) + "]"
}

func FormatBytes(n int64) string {
	const mb = 1000 * 1000
	if n >= mb {
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	}
	const kb = 1000
	if n >= kb {
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	}
	return fmt.Sprintf("%d B", n)
}

func FormatByteProgressFixed(completed, total int64) string {
	const mb = 1_000_000.0
	return fmt.Sprintf("%.1f/%.1f MB", float64(completed)/mb, float64(total)/mb)
}
