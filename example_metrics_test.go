package evo_test

import (
	"context"
	"fmt"
	"io"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// ExampleTaskTiming reads where one Task's time went. Evo stamps every
// lifecycle boundary from the run's Clock; the caller times nothing.
func ExampleTaskTiming() {
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true, Clock: clock})
	task := out.Task("compile schema").Define(func(context.Context) error {
		clock.Advance(2 * time.Second)
		return nil
	})
	_ = out.Finish()
	timing := task.Snapshot().Timing
	fmt.Println(timing.Queued(), timing.Running(), timing.Total())
	// Output:
	// 0s 2s 2s
}

// ExampleRunMetrics reads the run-level aggregate Conclusion.Metrics
// derives: how work resolved and where the run's time went, with no
// summary arithmetic in the caller.
func ExampleRunMetrics() {
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true, Clock: clock, MaxConcurrency: 1})
	declared := make(chan struct{})
	gen := out.Task("generate schema").Define(func(context.Context) error {
		<-declared // this example's fake clock advances only once both Tasks exist
		clock.Advance(3 * time.Second)
		return nil
	})
	pkg := out.Task("package").After(gen)
	pkg.Verify(func(context.Context) (bool, error) { return true, nil })
	pkg.Define(func(context.Context) error { return nil })
	close(declared)
	_ = out.Finish()
	m := out.Conclusion().Metrics()
	fmt.Println(m.Executed, m.AlreadySatisfied, m.Running, m.DependencyWait)
	// Output:
	// 1 1 3s 3s
}

// ExamplePhaseTime reads how long a Task spent inside its Define callback.
// Evo times the phase itself; the callback records nothing.
func ExamplePhaseTime() {
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true, Clock: clock})
	task := out.Task("render docs").Define(func(context.Context) error {
		clock.Advance(2 * time.Second)
		return nil
	})
	_ = out.Finish()
	definition := task.Snapshot().Timing.Definition
	fmt.Println(definition.Entries, definition.Duration)
	// Output:
	// 1 2s
}

// ExampleOperationCounts reads the §39 rates Evo derives from a run's
// tracked-operation tallies: here three of four operations were proven
// current by the manifest, and the one that ran produced identical output.
func ExampleOperationCounts() {
	ops := evo.OperationCounts{Current: 3, Executed: 1, Unchanged: 1}
	fmt.Println(ops.HitRate(), ops.PropagationStoppedRate())
	// Output:
	// 0.75 1
}
