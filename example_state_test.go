package evo_test

import (
	"context"
	"fmt"
	"io"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleEntityState shows the lifecycle state of an item or task, read
// from a resolved TaskSnapshot.
func ExampleEntityState() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	task := out.Task("apply patch")
	task.Define(func(context.Context) error { return nil })
	_ = out.Finish()
	state := task.Snapshot().State
	fmt.Println(state == evo.Done)
	// Output:
	// true
}

// ExampleConclusionState shows the human headline for a finished output.
func ExampleConclusionState() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	out.Task("apply patch").Define(func(context.Context) error { return nil })
	_ = out.Finish()
	fmt.Println(out.Conclusion().State == evo.StateReady)
	// Output:
	// true
}

// ExampleProgressKind classifies which measurement a task's Progress
// reports.
func ExampleProgressKind() {
	kind := evo.Determinate
	fmt.Println(kind == evo.Determinate)
	// Output:
	// true
}

// ExampleProgress shows the absolute measurement TaskHandle.Progress
// records, read back from the resolved TaskSnapshot.
func ExampleProgress() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	task := out.Task("download image")
	task.Progress(50, 100)
	task.Define(func(context.Context) error { return nil })
	_ = out.Finish()
	p := task.Snapshot().Progress
	fmt.Println(p.Completed, p.Total)
	// Output:
	// 50 100
}

// ExampleVisibility selects whether a message is ordinary or verbose user
// detail — zero is VisibilityNormal.
func ExampleVisibility() {
	var v evo.Visibility
	fmt.Println(v == evo.VisibilityNormal)
	// Output:
	// true
}
