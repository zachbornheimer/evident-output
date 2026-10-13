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
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	out.Task("apply patch").Define(func(context.Context) error { return nil })
	_ = out.finish()
	c := out.conclusion()
	fmt.Println(c.state, c.exitCode)
	// Output:
	// ready 0
}

// ExampleResolution names why a Task settled successfully.
func ExampleResolution() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	task := out.Task("apply patch")
	task.Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Println(task.snapshot().Resolution == resolutionExecuted)
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

// ExampleFact records a discovered name/value annotation about the run
// itself.
func ExampleFact() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	setDefault(out)
	fact("language", "go")
	_ = out.finish()
	fmt.Println(len(out.snapshot().Facts) == 1, out.snapshot().Facts[0].Value)
	// Output:
	// true go
}

// ExampleWarn annotates the run itself with a warning, without resolving
// any task.
func ExampleWarn() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	setDefault(out)
	out.Task("cache").Problem("cache directory missing, rebuilding", severity(severityWarning))
	_ = out.finish()
	fmt.Println(out.conclusion().warned)
	// Output:
	// true
}
