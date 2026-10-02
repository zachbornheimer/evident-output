package extract_test

// Removal and shape tests for evo.Extract. Extract is new in ZYS-1382 and
// nothing in 1.1 was an archive producer, so there is no 1.1 identifier
// that disappears because of Extract itself. What the migration does change
// is the identifier Extract is built on: Extract.File must be the plain
// File struct, which replaces the 1.1 func File(ctx, FileSpec) and the
// FileSpec type. These tests load the package with go/types (importer
// "source") so they compile and fail today instead of failing to build.

import (
	"go/importer"
	"go/token"
	"go/types"
	"sync"
	"testing"
)

const evoImportPath = "github.com/zachbornheimer/evident-output"

var (
	evoOnce sync.Once
	evoPkg  *types.Package
	evoErr  error
)

func loadEvo(t *testing.T) *types.Package {
	t.Helper()
	evoOnce.Do(func() {
		evoPkg, evoErr = importer.ForCompiler(token.NewFileSet(), "source", nil).Import(evoImportPath)
	})
	if evoErr != nil {
		t.Fatalf("load %s: %v", evoImportPath, evoErr)
	}
	return evoPkg
}

func lookup(t *testing.T, name string) types.Object {
	t.Helper()
	return loadEvo(t).Scope().Lookup(name)
}

func requireStruct(t *testing.T, name string) *types.Struct {
	t.Helper()
	obj := lookup(t, name)
	if obj == nil {
		t.Fatalf("evo.%s does not exist", name)
	}
	tn, ok := obj.(*types.TypeName)
	if !ok {
		t.Fatalf("evo.%s is a %T, want a type", name, obj)
	}
	if tn.IsAlias() {
		t.Fatalf("evo.%s is an alias (%v); want a plain declared struct", name, tn.Type())
	}
	st, ok := tn.Type().Underlying().(*types.Struct)
	if !ok {
		t.Fatalf("evo.%s underlying type is %v, want struct", name, tn.Type().Underlying())
	}
	return st
}

func fieldsOf(st *types.Struct) map[string]string {
	got := map[string]string{}
	for f := range st.Fields() {
		got[f.Name()] = types.TypeString(f.Type(), func(p *types.Package) string { return p.Name() })
	}
	return got
}

func TestRemoved_FileFuncAndFileSpecGiveWayToTheFileStruct(t *testing.T) {
	// Extract.File is the archive: it must be the File struct, so the 1.1
	// func File and FileSpec cannot still be what that name means.
	if obj := lookup(t, "File"); obj != nil {
		if _, isFunc := obj.(*types.Func); isFunc {
			t.Errorf("evo.File is still the 1.1 function %v; it must be the File struct", obj.Type())
		}
	}
	if obj := lookup(t, "FileSpec"); obj != nil {
		t.Errorf("evo.FileSpec still exists (%v); the 1.1 spec type is superseded by the File struct", obj.Type())
	}
}

func TestExtractIsAPlainStructWithExactFields(t *testing.T) {
	st := requireStruct(t, "Extract")
	fileStruct := requireStruct(t, "File")
	if fieldsOf(fileStruct)["Path"] != "string" {
		t.Fatalf("evo.File has no Path string field: %v", fieldsOf(fileStruct))
	}
	if st.NumFields() != 2 {
		t.Fatalf("evo.Extract fields = %v, want exactly File and Root", fieldsOf(st))
	}
	for field := range st.Fields() {
		switch f := field; f.Name() {
		case "File":
			if !types.Identical(f.Type(), lookup(t, "File").Type()) {
				t.Errorf("evo.Extract.File has type %v, want evo.File", f.Type())
			}
		case "Root":
			if f.Type().String() != "string" {
				t.Errorf("evo.Extract.Root has type %v, want string", f.Type())
			}
		default:
			t.Errorf("evo.Extract has unexpected field %s", f.Name())
		}
	}
}

func TestExtractIsATreeContentProducerNotAFileContentProducer(t *testing.T) {
	requireStruct(t, "Extract")
	extractType := lookup(t, "Extract").Type()
	for name, want := range map[string]bool{"TreeContent": true, "FileContent": false} {
		obj := lookup(t, name)
		if obj == nil {
			t.Fatalf("evo.%s does not exist", name)
		}
		iface, ok := obj.Type().Underlying().(*types.Interface)
		if !ok {
			t.Fatalf("evo.%s is %v, want an interface", name, obj.Type().Underlying())
		}
		if got := types.Implements(extractType, iface); got != want {
			t.Errorf("evo.Extract implements evo.%s = %v, want %v", name, got, want)
		}
	}
}

func TestTreeContentIsSealed(t *testing.T) {
	obj := lookup(t, "TreeContent")
	if obj == nil {
		t.Fatal("evo.TreeContent does not exist")
	}
	iface, ok := obj.Type().Underlying().(*types.Interface)
	if !ok {
		t.Fatalf("evo.TreeContent is %v, want an interface", obj.Type().Underlying())
	}
	if iface.NumMethods() == 0 {
		t.Fatal("evo.TreeContent has no methods; an empty interface is not sealed")
	}
	for m := range iface.Methods() {
		if m.Exported() {
			t.Errorf("evo.TreeContent exposes method %s; callers must not be able to implement it", m.Name())
		}
	}
}

func TestExtractErrorsExistAsErrors(t *testing.T) {
	for _, name := range []string{"ErrExtractMalformed", "ErrExtractUnsafeEntry"} {
		obj := lookup(t, name)
		v, ok := obj.(*types.Var)
		if !ok {
			t.Errorf("evo.%s = %v, want a package-level error variable", name, obj)
			continue
		}
		if v.Type().String() != "error" {
			t.Errorf("evo.%s has type %v, want error", name, v.Type())
		}
	}
}
