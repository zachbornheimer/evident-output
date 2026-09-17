package evo_test

import (
	"fmt"
	"os"
	"path/filepath"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleFileSpec declares one managed-state file resource — constructing
// it performs no I/O; passing it to File is what creates, rewrites on
// drift, or no-ops when Path/Contents/Mode already match.
func ExampleFileSpec() {
	dir, err := os.MkdirTemp("", "evo-example-filespec")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()

	spec := evo.FileSpec{
		Path:     filepath.Join(dir, "config.json"),
		Contents: []byte(`{"ok":true}`),
		Mode:     0o644,
	}
	fmt.Println(spec.Path != "")
	// Output:
	// true
}
