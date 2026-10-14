package evo

import (
	"context"
	"fmt"
	"io"
)

// ExampleOutcome shows a Task's Outcome and the run's Conclusion.Outcome
// for a Define that returned nil.
func ExampleOutcome() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true})
	task := out.Task("apply patch")
	task.Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Println(task.snapshot().State.Outcome(), out.conclusion().Outcome)
	// Output:
	// succeeded succeeded
}

// ExampleProgressKind classifies which measurement a task's Progress
// reports.
func ExampleProgressKind() {
	kind := determinate
	fmt.Println(kind == determinate)
	// Output:
	// true
}

// ExampleProgress shows the absolute measurement Task.Progress
// records, read back from the resolved TaskSnapshot.
func ExampleProgress() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true})
	task := out.Task("download image")
	task.Progress(50, 100)
	task.Define(func(context.Context) error { return nil })
	_ = out.finish()
	p := task.snapshot().Progress
	fmt.Println(p.Completed, p.Total)
	// Output:
	// 50 100
}

// ExampleVisibility selects whether a message is ordinary or verbose user
// detail — zero is VisibilityNormal.
func ExampleVisibility() {
	var v visibility
	fmt.Println(v == visibilityNormal)
	// Output:
	// true
}
