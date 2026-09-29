package render

import (
	"slices"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// A Task's own Facts (TaskHandle.Fact) are routine context, not results
// (contract §13: "normal successful Task    hide ordinary Facts ...
// verbose    show Task/run Facts"). They are not promoted when the Task
// warns or fails either: "A generic Task-scoped or run-scoped Fact is not
// promoted merely because some Task failed" (§13); "Unrelated configuration
// Facts stay hidden in normal mode even if the Task fails" (§21). Facts
// that explain a failure travel on the failing operation's
// VerificationDetail and render with it. Machine output never calls this.

// TaskAtVerbosity is t as a human reader sees it: its Facts kept only under
// verbose.
func TaskAtVerbosity(t core.TaskSnapshot, verbose bool) core.TaskSnapshot {
	if !verbose {
		t.Facts = nil
	}
	return t
}

// SnapshotAtVerbosity applies TaskAtVerbosity to every Task in s, at the
// root and in every collection. The input is not modified.
func SnapshotAtVerbosity(s core.Snapshot, verbose bool) core.Snapshot {
	if verbose {
		return s
	}
	s.Tasks = tasksAtVerbosity(s.Tasks, verbose)
	s.Collections = collectionsAtVerbosity(s.Collections, verbose)
	return s
}

func tasksAtVerbosity(tasks []core.TaskSnapshot, verbose bool) []core.TaskSnapshot {
	out := slices.Clone(tasks)
	for i := range out {
		out[i] = TaskAtVerbosity(out[i], verbose)
	}
	return out
}

func collectionsAtVerbosity(cols []core.TasksSnapshot, verbose bool) []core.TasksSnapshot {
	out := slices.Clone(cols)
	for i := range out {
		out[i].Tasks = tasksAtVerbosity(out[i].Tasks, verbose)
		out[i].Collections = collectionsAtVerbosity(out[i].Collections, verbose)
	}
	return out
}
