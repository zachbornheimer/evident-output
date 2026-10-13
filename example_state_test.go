package evo

import (
	"context"
	"fmt"
	"io"
)

// ExampleEntityState shows the lifecycle state of an item or task, read
// from a resolved TaskSnapshot.
func ExampleEntityState() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	task := out.Task("apply patch")
	task.Define(func(context.Context) error { return nil })
	_ = out.finish()
	state := task.snapshot().State
	fmt.Println(state == done)
	// Output:
	// true
}

// ExampleConclusionState shows the human headline for a finished output.
func ExampleConclusionState() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	out.Task("apply patch").Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Println(out.conclusion().state == stateReady)
	// Output:
	// true
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
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
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
