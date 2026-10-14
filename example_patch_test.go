package evo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/zachbornheimer/evident-output/internal/change"
)

// createNotesDiff creates a file that does not exist in the workspace, so
// Patch observes its absence as the Basis and reads nothing else.
const createNotesDiff = `diff --git a/example-patch-notes.txt b/example-patch-notes.txt
new file mode 100644
--- /dev/null
+++ b/example-patch-notes.txt
@@ -0,0 +1,2 @@
+first
+second
`

// ExamplePatch applies a unified diff inside a Task: it derives the desired
// file states and commits each through File. Forms that cannot reduce to a
// desired file state, such as deletion, fail explicitly.
func ExamplePatch() {
	dir, err := os.MkdirTemp("", "evo-example-patch")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()
	restore, err := chdir(dir)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer restore()

	out := initRun(Config{stateDir: dir, stdout: io.Discard, stderr: io.Discard})
	task := out.Task("apply notes")
	task.Define(func(ctx context.Context) error {
		err := Patch(ctx, []byte(createNotesDiff))
		fmt.Println("create:", err)

		err = Patch(ctx, []byte("diff --git a/go.mod b/go.mod\ndeleted file mode 100644\n"))
		fmt.Println("delete unsupported:", errors.Is(err, change.ErrPatchDeleteUnsupported))
		return nil
	})
	_ = task.wait()
	_ = out.finish()
	written, _ := os.ReadFile(filepath.Join(dir, "example-patch-notes.txt"))
	fmt.Printf("written: %q\n", written)
	// Output:
	// create: <nil>
	// delete unsupported: true
	// written: "first\nsecond\n"
}

// ExampleFileSet is what Patch returns: opaque desired file states, each
// bound to the Basis it was derived from, with no accessor that could
// separate the contents from that Basis.
func ExampleFileSet() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard})
	task := out.Task("derive notes")
	task.Define(func(ctx context.Context) error {
		var set fileSet
		set, err := derivePatch(ctx, []byte(createNotesDiff))
		_ = set // opaque: no field or method unpacks it
		fmt.Println("derived:", err == nil)
		return err
	})
	_ = task.wait()
	_ = out.finish()
	// Output:
	// derived: true
}

// ExampleFiles commits a Patch-derived FileSet through File. A file edited
// after Patch derived it is stale: Files refuses to overwrite it. Diff
// paths are relative to the workspace, the working directory at Run start.
func ExampleFiles() {
	dir, err := os.MkdirTemp("", "evo-example-files")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()
	restore, err := chdir(dir)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer restore()
	if err := os.WriteFile("notes.txt", []byte("draft\n"), 0o644); err != nil {
		fmt.Println(err)
		return
	}
	const diff = "--- a/notes.txt\n+++ b/notes.txt\n@@ -1 +1 @@\n-draft\n+final\n"

	out := initRun(Config{stateDir: dir, stdout: io.Discard, stderr: io.Discard})
	task := out.Task("finalize notes")
	task.Define(func(ctx context.Context) error {
		set, err := derivePatch(ctx, []byte(diff))
		if err != nil {
			return err
		}
		_ = os.WriteFile("notes.txt", []byte("edited meanwhile\n"), 0o644)
		fmt.Println("stale:", errors.Is(files(ctx, set), ErrStaleBasis))

		_ = os.WriteFile("notes.txt", []byte("draft\n"), 0o644)
		set, err = derivePatch(ctx, []byte(diff))
		if err != nil {
			return err
		}
		return files(ctx, set)
	})
	fmt.Println("applied:", task.wait())
	_ = out.finish()
	contents, _ := os.ReadFile(filepath.Join(dir, "notes.txt"))
	fmt.Print("notes: ", string(contents))
	// Output:
	// stale: true
	// applied: <nil>
	// notes: final
}

// chdir makes dir the working directory and returns a func restoring the
// previous one.
func chdir(dir string) (restore func(), err error) {
	prev, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := os.Chdir(dir); err != nil {
		return nil, err
	}
	return func() { _ = os.Chdir(prev) }, nil
}
