package evo

import (
	"context"
	"fmt"
	"io"
)

// ExampleCompute defines a Task that produces a value. A later Sequence
// step reads the value with Get once the earlier step has settled.
func ExampleCompute() {
	out := initRun(Config{plain: true, stdout: io.Discard})
	steps := out.Sequence("prune")
	branches := Compute(steps.Task("prune landed branches"), func(context.Context) ([]string, error) {
		return []string{"feat/a", "feat/b"}, nil
	})
	steps.Task("prune deleted remote branches").Define(func(context.Context) error {
		fmt.Println("pruned", len(branches.Get()), "branches")
		return nil
	})
	_ = out.finish()
	// Output:
	// pruned 2 branches
}

// ExampleComputed reads an earlier Sequence step's value without After: the
// Sequence proves the earlier step settled before the later one starts.
func ExampleComputed() {
	out := initRun(Config{plain: true, stdout: io.Discard})
	steps := out.Sequence("consolidate packages")
	managers := Compute(steps.Task("detect package managers"), func(context.Context) ([]string, error) {
		return []string{"npm", "composer"}, nil
	})
	steps.Task("discover installed packages").Define(func(context.Context) error {
		fmt.Println(managers.Get())
		return nil
	})
	_ = out.finish()
	// Output:
	// [npm composer]
}
