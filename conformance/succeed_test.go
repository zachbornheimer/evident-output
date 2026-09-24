package conformance_test

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

// succeed resolves task through the 1.1 success path — an empty Define,
// since ZYS-812 removed the TaskHandle.Done stamp — with summary as its
// optional result metadata (TaskHandle.Summary). It waits, so the row is
// terminal when succeed returns, as the removed Done was.
func succeed(task *evo.TaskHandle, summary ...string) {
	if len(summary) > 0 {
		task.Summary(summary[0])
	}
	_ = task.Define(func(context.Context) error { return nil }).Wait()
}
