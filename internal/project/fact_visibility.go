package project

import (
	"slices"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// A Task's own Facts (Task.Fact) are routine context, not results: CONTRACT
// §8 hides them at normal verbosity and always keeps them in machine output.
// They are not promoted when the Task warns or fails either; a Task-scoped
// Fact stays hidden in normal mode even if the Task fails. Facts that explain
// a failure travel on the failing operation's VerificationDetail and render
// with it. Machine output never calls this.

// TaskAtVerbosity is t as a human reader sees it: its Facts kept only under
// verbose.
func TaskAtVerbosity(t record.TaskSnapshot, verbose bool) record.TaskSnapshot {
	if !verbose {
		t.Facts = nil
	}
	return t
}

// SnapshotAtVerbosity applies TaskAtVerbosity to every Task in s, at the
// root and in every collection. The input is not modified.
func SnapshotAtVerbosity(s record.Snapshot, verbose bool) record.Snapshot {
	if verbose {
		return s
	}
	s.Tasks = tasksAtVerbosity(s.Tasks, verbose)
	s.Collections = collectionsAtVerbosity(s.Collections, verbose)
	return s
}

func tasksAtVerbosity(tasks []record.TaskSnapshot, verbose bool) []record.TaskSnapshot {
	if !slices.ContainsFunc(tasks, func(t record.TaskSnapshot) bool { return t.Facts != nil }) {
		return tasks // nothing to hide, so the caller's slice is already the answer
	}
	out := slices.Clone(tasks)
	for i := range out {
		out[i] = TaskAtVerbosity(out[i], verbose)
	}
	return out
}

func collectionsAtVerbosity(cols []record.TasksSnapshot, verbose bool) []record.TasksSnapshot {
	out := slices.Clone(cols)
	for i := range out {
		out[i].Tasks = tasksAtVerbosity(out[i].Tasks, verbose)
		out[i].Collections = collectionsAtVerbosity(out[i].Collections, verbose)
	}
	return out
}
