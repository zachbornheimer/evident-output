package evo_test

import (
	"fmt"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleResource shows the two ways to name shared state. A Resource
// prints as the caller wrote it; resolution to a canonical identity
// happens when an operation claims it.
func ExampleResource() {
	for _, r := range []evo.Resource{
		evo.FSResource("go.mod"),
		evo.LogicalResource("homebrew"),
	} {
		fmt.Println(r)
	}
	// Output:
	// fs:go.mod
	// logical:homebrew
}

// ExampleFSResource names a whole module directory: a coarse claim that
// overlaps every file beneath it, for an opaque operation that touches the
// module as a unit.
func ExampleFSResource() {
	module := evo.FSResource("./internal/engine")
	fmt.Println(module)
	// Output:
	// fs:./internal/engine
}

// ExampleLogicalResource names shared state that has no truthful
// filesystem path.
func ExampleLogicalResource() {
	remote := evo.LogicalResource("git remote origin")
	fmt.Println(remote)
	// Output:
	// logical:git remote origin
}
