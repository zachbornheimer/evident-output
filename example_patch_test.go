package evo_test

import (
	"context"
	"errors"
	"fmt"
	"io"

	evo "github.com/zachbornheimer/evident-output"
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

// ExamplePatch derives desired file states from a unified diff inside a
// Task and mutates nothing. Forms that cannot reduce to a desired file
// state, such as deletion, fail explicitly.
func ExamplePatch() {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Stderr: io.Discard})
	task := out.Task("derive notes")
	task.Define(func(ctx context.Context) error {
		_, err := evo.Patch(ctx, []byte(createNotesDiff))
		fmt.Println("create:", err)

		_, err = evo.Patch(ctx, []byte("diff --git a/go.mod b/go.mod\ndeleted file mode 100644\n"))
		fmt.Println("delete unsupported:", errors.Is(err, evo.ErrPatchDeleteUnsupported))
		return nil
	})
	_ = task.Wait()
	_ = out.Finish()
	// Output:
	// create: <nil>
	// delete unsupported: true
}

// ExampleFileSet is what Patch returns: opaque desired file states, each
// bound to the Basis it was derived from, with no accessor that could
// separate the contents from that Basis.
func ExampleFileSet() {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Stderr: io.Discard})
	task := out.Task("derive notes")
	task.Define(func(ctx context.Context) error {
		var set evo.FileSet
		set, err := evo.Patch(ctx, []byte(createNotesDiff))
		_ = set // opaque: no field or method unpacks it
		fmt.Println("derived:", err == nil)
		return err
	})
	_ = task.Wait()
	_ = out.Finish()
	// Output:
	// derived: true
}
