package evo_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// A Run is the root of one invocation: it declares the work, carries the
// run-scoped Fact and Summary, and is handed to the body by Execute and Main.
func ExampleRun() {
	var out bytes.Buffer
	evo.Execute(context.Background(), quietRun(&out), func(ctx context.Context, r *evo.Run) error {
		r.Summary("release 1.4")
		r.Fact("branch", "main")
		r.Task("tag").Define(func(context.Context) error { return nil })
		return nil
	})
	fmt.Print(out.String())
	// Output:
	// release 1.4
	// branch  main
	// ✓ tag
	//
	// [succeeded]  demo
}

// A Task is one atomic promise. Verify short-circuits it when the world is
// already right; Define carries the work.
func ExampleTask() {
	var out bytes.Buffer
	evo.Execute(context.Background(), quietRun(&out), func(ctx context.Context, r *evo.Run) error {
		r.Task("create output directory").
			Verify(func(context.Context) (bool, error) { return true, nil }).
			Define(func(context.Context) error { return errors.New("not reached: Verify already holds") })
		return nil
	})
	fmt.Print(out.String())
	// Output:
	// ✓ create output directory  already satisfied
	//
	// [satisfied]  demo
}

// A Group's children are independent: eligible siblings may overlap.
func ExampleGroup() {
	var out bytes.Buffer
	evo.Execute(context.Background(), quietRun(&out), func(ctx context.Context, r *evo.Run) error {
		repo := r.Group("repository")
		repo.Task("fetch").Define(func(context.Context) error { return nil })
		repo.Task("prune").Define(func(context.Context) error { return nil })
		return nil
	})
	fmt.Print(out.String())
	// Output:
	// ✓ repository
	//    ✓ fetch
	//    ✓ prune
	//
	// [succeeded]  demo
}

// A Sequence runs its children in declaration order, one at a time.
func ExampleSequence() {
	var out bytes.Buffer
	evo.Execute(context.Background(), quietRun(&out), func(ctx context.Context, r *evo.Run) error {
		steps := r.Sequence("release")
		for _, name := range []string{"test", "build", "publish"} {
			steps.Task(name).Define(func(context.Context) error { return nil })
		}
		return nil
	})
	fmt.Print(out.String())
	// Output:
	// ✓ release
	//    ✓ test
	//    ✓ build
	//    ✓ publish
	//
	// [succeeded]  demo
}

// After accepts a Predecessor: a Task, Group, Sequence or Computed. The
// dependent waits for it and stays NotStarted if it does not succeed.
func ExamplePredecessor() {
	var out bytes.Buffer
	evo.Execute(context.Background(), quietRun(&out), func(ctx context.Context, r *evo.Run) error {
		build := r.Task("build")
		build.Define(func(context.Context) error { return nil })
		var ready evo.Predecessor = build
		r.Task("deploy").After(ready).Define(func(context.Context) error { return nil })
		return nil
	})
	fmt.Print(out.String())
	// Output:
	// ✓ build
	// ✓ deploy
	//
	// [succeeded]  demo
}

// A DefineOption adjusts how Define runs its callback.
func ExampleDefineOption() {
	var out bytes.Buffer
	opts := []evo.DefineOption{evo.CleanStop(syscall.SIGTERM)}
	evo.Execute(context.Background(), quietRun(&out), func(ctx context.Context, r *evo.Run) error {
		r.Task("serve").Define(func(context.Context) error { return nil }, opts...)
		return nil
	})
	fmt.Print(out.String())
	// Output:
	// ✓ serve
	//
	// [succeeded]  demo
}

// CleanStop names the signals that end a long-running Task Succeeded instead
// of cancelling the run, so a server stopped with Ctrl-C exits 0.
func ExampleCleanStop() {
	var out bytes.Buffer
	serve := func(ctx context.Context) error {
		select {
		case <-ctx.Done(): // a named signal arrived
		case <-time.After(time.Millisecond): // this example's server stops on its own
		}
		return nil
	}
	evo.Execute(context.Background(), quietRun(&out), func(ctx context.Context, r *evo.Run) error {
		r.Task("serve").Define(serve, evo.CleanStop(os.Interrupt, syscall.SIGTERM))
		return nil
	})
	fmt.Print(out.String())
	// Output:
	// ✓ serve
	//
	// [succeeded]  demo
}
