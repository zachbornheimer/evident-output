package evo

import (
	"context"
	"fmt"
	"io"
)

// ExampleGroup_name shows the collection handle Group returns: it declares
// its own children and reports a collection-level Snapshot.
func ExampleGroup_name() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true})
	group := out.Group("packages")
	group.Task("curl").Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Println(group.snapshot().Name)
	// Output:
	// packages
}

// ExampleContainer mounts the same topology builder on a run, a Group, and a Sequence.
func ExampleContainer() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true})
	mount := func(c Container) {
		c.Task("check").Define(func(context.Context) error { return nil })
	}
	mount(out)
	mount(out.Group("g"))
	mount(out.Sequence("s"))
	_ = out.finish()
	fmt.Println("ok")
	// Output:
	// ok
}

// ExampleSequence_name shows the ordered collection handle Sequence
// returns.
func ExampleSequence_name() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true})
	seq := out.Sequence("stages")
	task := seq.Task("build")
	task.Define(func(ctx context.Context) error { return nil })
	_ = out.finish()
	fmt.Println(seq.snapshot().Name)
	// Output:
	// stages
}

// ExampleTasksSnapshot reads a collection's immutable view: its own state
// plus every child Task it declared.
func ExampleTasksSnapshot() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true})
	group := out.Group("packages")
	group.Task("curl").Define(func(context.Context) error { return nil })
	_ = out.finish()
	snap := group.snapshot()
	fmt.Println(snap.Name, len(snap.Tasks))
	// Output:
	// packages 1
}
