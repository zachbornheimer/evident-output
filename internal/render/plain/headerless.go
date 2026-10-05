package plain

import (
	"strings"

	"github.com/zachbornheimer/evident-output/internal/render"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// writeHeaderlessGroup renders a Group's children as siblings of the
// surrounding rows, aligned to one name column, then any nested containers
// the same way.
func writeHeaderlessGroup(b *strings.Builder, col core.TasksSnapshot, s render.Style) {
	nameWidth := render.HeaderlessRowNameWidth(col)
	for _, t := range col.Tasks {
		render.WriteTaskAligned(b, t, nameWidth, s)
	}
	for _, child := range col.Collections {
		writeCollectionAligned(b, child, nameWidth, s)
	}
}
