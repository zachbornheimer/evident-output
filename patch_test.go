package evo_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestFileSetCannotBeUnpacked proves FileSet exposes nothing a caller could
// use to copy desired contents out while dropping their Basis: no exported
// field and no exported method.
func TestFileSetCannotBeUnpacked(t *testing.T) {
	typ := reflect.TypeFor[evo.FileSet]()
	if typ.Kind() != reflect.Struct {
		t.Fatalf("FileSet is a %v, want an opaque struct", typ.Kind())
	}
	for i := range typ.NumField() {
		if field := typ.Field(i); field.IsExported() {
			t.Errorf("FileSet exports field %s", field.Name)
		}
	}
	for _, methods := range []reflect.Type{typ, reflect.PointerTo(typ)} {
		for i := range methods.NumMethod() {
			t.Errorf("%v exports method %s", methods, methods.Method(i).Name)
		}
	}
}

// TestPatchThroughPublicAPI derives a FileSet through evo.Patch from a Task
// and proves the workspace is untouched and unsupported forms fail by
// their public sentinels.
func TestPatchThroughPublicAPI(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path := filepath.Join(dir, "greeting.txt")
	if err := os.WriteFile(path, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := evo.Init(evo.Config{Isolated: true, StateDir: t.TempDir(), Stdout: io.Discard, Stderr: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	derived := 0
	derive := func(diff string) error {
		derived++
		var patchErr error
		task := out.Task(fmt.Sprintf("derive patch %d", derived))
		task.Define(func(ctx context.Context) error {
			_, patchErr = evo.Patch(ctx, []byte(diff))
			return nil
		})
		_ = task.Wait()
		return patchErr
	}

	modify := "--- a/greeting.txt\n+++ b/greeting.txt\n@@ -1,2 +1,2 @@\n hello\n-world\n+there\n"
	if err := derive(modify); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "hello\nworld\n" {
		t.Fatalf("Patch mutated %s: %q", path, got)
	}
	unsupported := map[string]error{
		"diff --git a/greeting.txt b/greeting.txt\ndeleted file mode 100644\n":                              evo.ErrPatchDeleteUnsupported,
		"diff --git a/greeting.txt b/g.txt\nrename from greeting.txt\nrename to g.txt\n":                    evo.ErrPatchRenameUnsupported,
		"diff --git a/greeting.txt b/greeting.txt\nBinary files a/greeting.txt and b/greeting.txt differ\n": evo.ErrPatchBinaryUnsupported,
	}
	for diff, want := range unsupported {
		if err := derive(diff); !errors.Is(err, want) || !errors.Is(err, evo.ErrPatchUnsupported) {
			t.Errorf("Patch(%q) = %v, want %v", diff, err, want)
		}
	}
	if _, err := evo.Patch(context.Background(), []byte(modify)); !errors.Is(err, evo.ErrNoTaskContext) {
		t.Fatalf("Patch outside a Task = %v, want ErrNoTaskContext", err)
	}
}
