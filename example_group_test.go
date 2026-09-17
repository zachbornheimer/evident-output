package evo_test

import (
	"bytes"
	"context"
	"fmt"
	"io"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleGroup declares an independent, concurrent collection of child
// Tasks on the default instance.
func ExampleGroup() {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Stdout: &buf, Stderr: io.Discard, Plain: true}))
	install := evo.Group("install")
	install.Task("curl").Done()
	_ = evo.Default().Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ install
	//    ✓ curl
}

// ExampleGroupHandle shows the collection handle Group returns: it declares
// its own children and reports a collection-level Snapshot.
func ExampleGroupHandle() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	group := out.Group("packages")
	group.Task("curl").Done()
	_ = out.Finish()
	fmt.Println(group.Snapshot().Name)
	// Output:
	// packages
}

// ExampleSequence declares a self-managing, ordered task container: one
// Running child at a time, in declaration order.
func ExampleSequence() {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Stdout: &buf, Stderr: io.Discard, Plain: true}))
	worktrees := evo.Sequence("worktrees")
	for _, path := range []string{"repo-a", "repo-b"} {
		task := worktrees.Task(path)
		task.Define(func(ctx context.Context) error { return nil })
	}
	_ = evo.Default().Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ worktrees
	//    ✓ repo-a
	//    ✓ repo-b
}

// ExampleSequenceHandle shows the ordered collection handle Sequence
// returns.
func ExampleSequenceHandle() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	seq := out.Sequence("stages")
	task := seq.Task("build")
	task.Define(func(ctx context.Context) error { return nil })
	_ = out.Finish()
	fmt.Println(seq.Snapshot().Name)
	// Output:
	// stages
}

// ExampleTasksSnapshot reads a collection's immutable view: its own state
// plus every child Task it declared.
func ExampleTasksSnapshot() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	group := out.Group("packages")
	group.Task("curl").Done()
	_ = out.Finish()
	snap := group.Snapshot()
	fmt.Println(snap.Name, len(snap.Tasks))
	// Output:
	// packages 1
}
