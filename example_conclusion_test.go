package evo_test

import (
	"fmt"
	"io"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleConclusion shows the multidimensional meaning of a finished
// command, read from a real run.
func ExampleConclusion() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	out.Task("apply patch").Done()
	_ = out.Finish()
	c := out.Conclusion()
	fmt.Println(c.State, c.ExitCode)
	// Output:
	// ready 0
}

// ExampleResolution names why a Task settled successfully.
func ExampleResolution() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	task := out.Task("apply patch")
	task.Done()
	_ = out.Finish()
	fmt.Println(task.Snapshot().Resolution == evo.ResolutionNoWork)
	// Output:
	// true
}

// ExampleEvent shows the immutable journal record durable event streams
// carry.
func ExampleEvent() {
	e := evo.Event{Type: "task.done", Name: "apply patch", Timestamp: time.Unix(0, 0)}
	fmt.Println(e.Type, e.Name)
	// Output:
	// task.done apply patch
}

// ExampleFactRecord shows a discovered name/value annotation — information,
// not work.
func ExampleFactRecord() {
	f := evo.FactRecord{Name: "language", Value: "go"}
	fmt.Println(f.Name, f.Value)
	// Output:
	// language go
}

// ExampleFact records a discovered name/value annotation about the run
// itself.
func ExampleFact() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	evo.SetDefault(out)
	evo.Fact("language", "go")
	_ = out.Finish()
	fmt.Println(len(out.Snapshot().Facts) == 1, out.Snapshot().Facts[0].Value)
	// Output:
	// true go
}

// ExampleWarn annotates the run itself with a warning, without resolving
// any task.
func ExampleWarn() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	evo.SetDefault(out)
	evo.Warn("cache directory missing, rebuilding")
	_ = out.Finish()
	fmt.Println(out.Conclusion().Warned)
	// Output:
	// true
}

// ExampleDelay returns a non-nil *time.Duration for Config fields where
// zero is meaningful, like VisibilityDelay.
func ExampleDelay() {
	d := evo.Delay(0)
	fmt.Println(*d)
	// Output:
	// 0s
}
