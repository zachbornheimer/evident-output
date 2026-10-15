package evo

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
)

// ExampleFSResource names a whole module directory: a coarse claim that
// overlaps every file beneath it, for an opaque operation that touches the
// module as a unit.
func ExampleFSResource() {
	module := change.FSResource("./internal/engine")
	fmt.Println(module)
	// Output:
	// fs:./internal/engine
}

// ExampleLogicalResource names shared state that has no truthful
// filesystem path.
func ExampleLogicalResource() {
	remote := change.LogicalResource("git remote origin")
	fmt.Println(remote)
	// Output:
	// logical:git remote origin
}
