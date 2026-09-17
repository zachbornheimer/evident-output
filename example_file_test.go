package evo_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
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

// ExampleFileFS is the facade every evo.File call performs filesystem I/O
// through instead of the os package directly — testkit.FileFS satisfies it
// deterministically for tests (including scripting a chmod failure);
// production uses the real filesystem.
func ExampleFileFS() {
	dir, err := os.MkdirTemp("", "evo-example-filefs")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "config.json")

	fsys := testkit.NewFileFS()
	out := evo.Init(evo.Config{
		Isolated: true, FileFS: fsys, Plain: true, Color: evo.ColorNever,
		// StateDir isolates this example's manifest to its own temp
		// directory (spec §11.3) — without it, File's default manifest
		// path is derived from the real machine cache dir and can
		// contend with any other concurrently running evo.File caller.
		StateDir: dir,
		Stdout:   io.Discard, Stderr: io.Discard,
	})
	agent := out.Task("write config")
	agent.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte(`{"ok":true}`)})
	})
	_ = out.Finish()

	contents, err := os.ReadFile(path)
	fmt.Println(err == nil, string(contents))
	// Output:
	// true {"ok":true}
}
