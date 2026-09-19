package evo_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	evo "github.com/zachbornheimer/evident-output"
)

// ExamplePatch derives desired file state from a unified diff without
// writing, then File commits that state.
func ExamplePatch() {
	dir, err := os.MkdirTemp("", "evo-example-patch")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("hello\nworld\n"), 0o644); err != nil {
		fmt.Println(err)
		return
	}

	out := evo.Init(evo.Config{
		Isolated: true, StateDir: dir, Plain: true, Color: evo.ColorNever,
		Stdout: io.Discard, Stderr: io.Discard,
	})
	task := out.Task("apply greeting")
	task.Define(func(ctx context.Context) error {
		result, err := evo.Patch(ctx, evo.PatchSpec{
			Diff: []byte("" +
				"--- a/hello.txt\n" +
				"+++ b/hello.txt\n" +
				"@@ -1,2 +1,2 @@\n" +
				" hello\n" +
				"-world\n" +
				"+there\n"),
			Dir: dir,
		})
		if err != nil {
			return err
		}
		for _, spec := range result.Files {
			if err := evo.File(ctx, spec); err != nil {
				return err
			}
		}
		return nil
	})
	_ = out.Finish()

	got, err := os.ReadFile(path)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Print(string(got))
	// Output:
	// hello
	// there
}
