package plain

import (
	"strings"

	"github.com/zachbornheimer/evident-output/internal/project"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// writeHeaderlessGroup renders a Group's children as siblings of the
// surrounding rows, aligned to one name column, then any nested containers
// the same way.
func writeHeaderlessGroup(b *strings.Builder, col record.TasksSnapshot, s project.Style) {
	nameWidth := project.HeaderlessRowNameWidth(col)
	for _, t := range col.Tasks {
		project.WriteTaskAligned(b, t, nameWidth, s)
	}
	for _, child := range col.Collections {
		writeCollectionAligned(b, child, nameWidth, s)
	}
}
