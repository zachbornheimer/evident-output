package evo

import (
	"bytes"
	"context"
	"fmt"
	"io"
)

// ExampleGroup declares an independent, concurrent collection of child
// Tasks on the default instance.
func ExampleGroup() {
	var buf bytes.Buffer
	setDefault(initRun(Config{stdout: &buf, stderr: io.Discard, plain: true}))
	install := defaultRun().Group("install")
	install.Task("curl").Define(func(context.Context) error { return nil })
	_ = defaultRun().finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ curl
}

// ExampleGroup_name shows the collection handle Group returns: it declares
// its own children and reports a collection-level Snapshot.
func ExampleGroup_name() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	group := out.Group("packages")
	group.Task("curl").Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Println(group.snapshot().Name)
	// Output:
	// packages
}

// ExampleContainer mounts the same topology builder on a run, a Group, and a Sequence.
func ExampleContainer() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
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

// ExampleSequence declares a self-managing, ordered task container: one
// Running child at a time, in declaration order.
func ExampleSequence() {
	var buf bytes.Buffer
	setDefault(initRun(Config{stdout: &buf, stderr: io.Discard, plain: true}))
	worktrees := defaultRun().Sequence("worktrees")
	for _, path := range []string{"repo-a", "repo-b"} {
		task := worktrees.Task(path)
		task.Define(func(ctx context.Context) error { return nil })
	}
	_ = defaultRun().finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ worktrees
	//    ✓ repo-a
	//    ✓ repo-b
}

// ExampleSequence_name shows the ordered collection handle Sequence
// returns.
func ExampleSequence_name() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	seq := out.Sequence("stages")
	task := seq.Task("build")
	task.Define(func(ctx context.Context) error { return nil })
	_ = out.finish()
	fmt.Println(seq.snapshot().Name)
	// Output:
	// stages
}

// ExampleGroup_Wait shows the declare-all-then-wait pattern (ZYS-849):
// after every child Task is declared, the Group itself is the one thing a
// caller waits on and asks for the aggregate outcome — no caller-owned task
// slice, no hand-counted failures, no Snapshot walk.
func exampleGroup_Wait() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	jobs := out.Group("check")
	for _, name := range []string{"lint", "vet", "test"} {
		task := jobs.Task(name)
		task.Define(func(ctx context.Context) error { return nil })
	}
	err := jobs.wait()
	_ = out.finish()
	fmt.Println(err)
	// Output:
	// <nil>
}

// ExampleSequence_Wait is Sequence's counterpart to
// ExampleGroup_Wait: the ordered container is awaited directly once
// every step is declared, and ordering is still preserved underneath.
func exampleSequence_Wait() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	python := out.Sequence("python")
	for _, name := range []string{"scan", "venv", "install"} {
		task := python.Task(name)
		task.Define(func(ctx context.Context) error { return nil })
	}
	err := python.wait()
	_ = out.finish()
	fmt.Println(err)
	// Output:
	// <nil>
}

// ExampleTasksSnapshot reads a collection's immutable view: its own state
// plus every child Task it declared.
func ExampleTasksSnapshot() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	group := out.Group("packages")
	group.Task("curl").Define(func(context.Context) error { return nil })
	_ = out.finish()
	snap := group.snapshot()
	fmt.Println(snap.Name, len(snap.Tasks))
	// Output:
	// packages 1
}
