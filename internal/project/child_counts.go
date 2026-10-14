package project

import (
	"slices"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// HasUnfinishedTask reports whether any Task at or below col is still
// pending or running. While work is in flight the live header carries the
// Group's own aggregate progress ("N/M complete", with its elapsed time);
// once every Task is terminal that count is spent.
func HasUnfinishedTask(col record.TasksSnapshot) bool {
	if OwnCounts(col).Unfinished || CollectionTallyOf(col).Tasks.Unfinished {
		return true
	}
	return slices.ContainsFunc(col.Collections, HasUnfinishedTask)
}

// OwnCounts summarizes col's own child Tasks, left-out ones included.
func OwnCounts(col record.TasksSnapshot) ChildCounts {
	if tally, ok := ChildTallyOf(col); ok {
		return tally.All
	}
	return CountTasks(col.Tasks)
}
