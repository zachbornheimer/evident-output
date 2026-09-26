package live

import (
	"strings"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render"
)

// writeLiveDispositions writes items' tallies at indent as the live frame
// shows them (never verbose) within maxRows, and reports how many rows they took, so
// the frame's height budget can count them. When the cause lines do not
// fit, each tally keeps its headline and drops its causes: the headline is
// the count, the durable render still carries the evidence.
func writeLiveDispositions(b *strings.Builder, indent string, items core.Dispositions, maxRows int, s render.Style) (rows int) {
	if items.Empty() {
		return 0
	}
	s.Verbose = false
	var full strings.Builder
	render.WriteDispositions(&full, indent, items, render.NoDisposition, s)
	if rows = strings.Count(full.String(), "\n"); rows <= maxRows {
		b.WriteString(full.String())
		return rows
	}
	start := b.Len()
	render.WriteTaxonomyHeadline(b, indent, render.DispositionSkipped, items.Skipped, s)
	render.WriteTaxonomyHeadline(b, indent, render.DispositionKept, items.Kept, s)
	return strings.Count(b.String()[start:], "\n")
}
