package evo

import (
	"context"
	"fmt"
	"io"
	"time"
)

// ExampleConclusion shows the multidimensional meaning of a finished
// command, read from a real run.
func ExampleConclusion() {
	out := initRun(Config{Output: Plain, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard}})
	out.Task("apply patch").Define(func(context.Context) error { return nil })
	_ = out.finish()
	c := out.conclusion()
	fmt.Println(c.headline(), c.exitCode)
	// Output:
	// succeeded 0
}

// ExampleResolution shows that a Task whose Define ran records it.
func ExampleResolution() {
	out := initRun(Config{Output: Plain, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard}})
	task := out.Task("apply patch")
	task.Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Println(task.snapshot().DefineRan)
	// Output:
	// true
}

// ExampleEvent shows the immutable journal record durable event streams
// carry.
func ExampleEvent() {
	e := recEvent{Type: "task.done", Name: "apply patch", Timestamp: time.Unix(0, 0)}
	fmt.Println(e.Type, e.Name)
	// Output:
	// task.done apply patch
}
