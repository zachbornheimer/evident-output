package render

import (
	"slices"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// A Group that renders without its header (flattensHeader) puts its rows
// beside the rows around it, and a Task's identity is its container path
// plus its name. So a flattened row whose bare name another visible row
// at the same level also shows names its container path instead
// ("g › build"), exactly as the ledger qualifies a section subject once
// two sections share a name. A unique name stays bare. Rows under a header
// are a level of their own: the header already says where they are.

// headerRule reports whether col, its disposition items already folded
// out, renders without its header. Plain and live differ only in this.
type headerRule func(col core.TasksSnapshot, items core.Dispositions) bool

// liveFlattensHeader is the live frame's rule: the header stays only
// while its aggregate "N/M complete" count says something the rows do not
// (liveProgressAddsInformation) — never "0/0 complete" for a Group that
// holds only nested Groups.
func liveFlattensHeader(col core.TasksSnapshot, items core.Dispositions) bool {
	_, total := completion(col)
	return flattensHeader(col, items) && !liveProgressAddsInformation(col, total)
}

// qualifyFlattenedRows returns s with every flattened row whose name
// collides at its level named by its container path. The input is not
// modified.
func qualifyFlattenedRows(s core.Snapshot, flattens headerRule) core.Snapshot {
	s.Collections = qualifyLevel(s.Tasks, s.Collections, flattens)
	return s
}

// qualifyLevel qualifies the collections rendered at one level beside the
// given Task rows.
func qualifyLevel(tasks []core.TaskSnapshot, cols []core.TasksSnapshot, flattens headerRule) []core.TasksSnapshot {
	if len(cols) == 0 {
		return cols
	}
	names := make(map[string]int, len(tasks)+len(cols))
	for _, t := range tasks {
		names[t.Name]++
	}
	for _, col := range cols {
		countLevelRows(col, names, flattens)
	}
	out := make([]core.TasksSnapshot, len(cols))
	for i, col := range cols {
		out[i] = qualifyCollection(col, "", names, flattens)
	}
	return out
}

// countLevelRows counts the row names col contributes to its parent's
// level: its own row, or, when it flattens, its children's.
func countLevelRows(col core.TasksSnapshot, names map[string]int, flattens headerRule) {
	rest, items := withoutDispositionItems(col)
	switch {
	case rendersAsOwnTask(rest):
		names[rest.Name]++
	case flattens(rest, items):
		for _, t := range rest.Tasks {
			names[t.Name]++
		}
		for _, child := range rest.Collections {
			countLevelRows(child, names, flattens)
		}
	default:
		names[col.Name]++
	}
}

// qualifyCollection rewrites col for its level. path is the container path
// of the flattened Groups above col at this level ("" at the top).
func qualifyCollection(col core.TasksSnapshot, path string, names map[string]int, flattens headerRule) core.TasksSnapshot {
	rest, items := withoutDispositionItems(col)
	switch {
	case rendersAsOwnTask(rest):
		return col
	case flattens(rest, items):
		path = qualifiedName(path, col.Name)
		col.Tasks = slices.Clone(col.Tasks)
		for i := range col.Tasks {
			if names[col.Tasks[i].Name] > 1 {
				col.Tasks[i].Name = qualifiedName(path, col.Tasks[i].Name)
			}
		}
		col.Collections = slices.Clone(col.Collections)
		for i, child := range col.Collections {
			col.Collections[i] = qualifyCollection(child, path, names, flattens)
		}
	default:
		col.Collections = qualifyLevel(rest.Tasks, col.Collections, flattens)
	}
	return col
}

func qualifiedName(path, name string) string {
	if path == "" {
		return name
	}
	return path + core.QualifiedSubjectSeparator + name
}
