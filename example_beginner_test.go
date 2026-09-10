package evo_test

import (
	"io"

	evo "github.com/zachbornheimer/evident-output"
)

func Example_beginnerAPI() {
	evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true})
	paths := []string{"../.worktrees/app-sah-1"}
	check := func(string) error { return nil }
	for path, task := range evo.Group("worktrees").Each(paths) {
		task.Define(func() error { return check(path) })
	}
	// Output:
}
