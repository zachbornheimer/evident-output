package evo_test

import (
	"context"
	"fmt"
	"io"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleCompute defines a Task that produces a value. The Task that runs
// After it reads the value with Get.
func ExampleCompute() {
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard})
	branches := evo.Compute(out.Task("prune landed branches"), func(context.Context) ([]string, error) {
		return []string{"feat/a", "feat/b"}, nil
	})
	out.Task("prune deleted remote branches").After(branches).Define(func(context.Context) error {
		fmt.Println("pruned", len(branches.Get()), "branches")
		return nil
	})
	_ = out.Finish()
	// Output:
	// pruned 2 branches
}

// ExampleComputed reads an earlier Sequence step's value without After: the
// Sequence proves the earlier step settled before the later one starts.
func ExampleComputed() {
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard})
	steps := out.Sequence("consolidate packages")
	managers := evo.Compute(steps.Task("detect package managers"), func(context.Context) ([]string, error) {
		return []string{"npm", "composer"}, nil
	})
	steps.Task("discover installed packages").Define(func(context.Context) error {
		fmt.Println(managers.Get())
		return nil
	})
	_ = out.Finish()
	// Output:
	// [npm composer]
}
