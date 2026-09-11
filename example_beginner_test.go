package evo_test

import (
	"bytes"
	"fmt"
	"io"

	evo "github.com/zachbornheimer/evident-output"
)

// Example_beginnerAPI proves the beginner shape compiles and renders real
// rows, not an empty // Output: that only proves compile (evo-dialect-axes-
// report.md axis 2: "the example discards stdout and asserts nothing").
// Sequence (declaration order, one Running child at a time) keeps the two
// child rows deterministic across runs; Group.Each would be correct too but
// collapses an all-success aggregate into a single summary line.
func Example_beginnerAPI() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Stdout: &buf, Stderr: io.Discard, Plain: true})
	check := func(string) error { return nil }
	worktrees := out.Sequence("worktrees")
	for _, path := range []string{"repo-a", "repo-b"} {
		task := worktrees.Task(path)
		task.Define(func() error { return check(path) })
	}
	if err := out.Finish(); err != nil {
		fmt.Println(err)
	}
	fmt.Print(buf.String())
	// Output:
	// ✓ worktrees
	//    ✓ repo-a
	//    ✓ repo-b
}
