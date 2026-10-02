package exec_test

// Removal and shape tests for the ZYS-1382 Exec migration. They inspect
// package evo through go/types (source importer) instead of referencing
// identifiers directly, so this file compiles whether or not the 1.1 shapes
// still exist. Run alone before the new surface lands:
//
//	go test ./conformance/zys1382/exec/removed_test.go
//
// They fail against 1.1 (ExecSpec, func Exec) and pass once the migration
// is real.

import (
	"go/importer"
	"go/token"
	"go/types"
	"slices"
	"sync"
	"testing"
)

const evoImportPath = "github.com/zachbornheimer/evident-output"

var loadEvo = sync.OnceValues(func() (*types.Package, error) {
	return importer.ForCompiler(token.NewFileSet(), "source", nil).Import(evoImportPath)
})

func evoScope(t *testing.T) *types.Scope {
	t.Helper()
	pkg, err := loadEvo()
	if err != nil {
		t.Fatalf("type-check package evo from source: %v", err)
	}
	return pkg.Scope()
}

// evoType returns the named type evo.<name>, failing when it is absent or
// not a type.
func evoType(t *testing.T, name string) *types.Named {
	t.Helper()
	obj := evoScope(t).Lookup(name)
	if obj == nil {
		t.Fatalf("evo.%s does not exist", name)
	}
	tn, ok := obj.(*types.TypeName)
	if !ok {
		t.Fatalf("evo.%s is a %T, want a type", name, obj)
	}
	named, ok := types.Unalias(tn.Type()).(*types.Named)
	if !ok {
		t.Fatalf("evo.%s is %s, want a named type", name, tn.Type())
	}
	return named
}

func evoStruct(t *testing.T, name string) *types.Struct {
	t.Helper()
	st, ok := evoType(t, name).Underlying().(*types.Struct)
	if !ok {
		t.Fatalf("evo.%s is %s, want a struct", name, evoType(t, name).Underlying())
	}
	return st
}

type field struct{ name, typ string }

// fields lists st's fields with types qualified by package name ("evo.Outputs").
func fields(st *types.Struct) []field {
	q := func(p *types.Package) string { return p.Name() }
	var out []field
	for v := range st.Fields() {
		out = append(out, field{v.Name(), types.TypeString(v.Type(), q)})
	}
	return out
}

func TestRemoved_ExecSpecType(t *testing.T) {
	if obj := evoScope(t).Lookup("ExecSpec"); obj != nil {
		t.Fatalf("evo.ExecSpec still exists (%s); ZYS-1382 replaces it with the evo.Exec struct", obj)
	}
}

func TestRemoved_ExecFunctionForm(t *testing.T) {
	obj := evoScope(t).Lookup("Exec")
	if fn, ok := obj.(*types.Func); ok {
		t.Fatalf("evo.Exec is still the function %s; ZYS-1382 makes it a struct with a Run method", fn.Type())
	}
}

func TestRemoved_ErrExecSpecMissingExecutable(t *testing.T) {
	if obj := evoScope(t).Lookup("ErrExecSpecMissingExecutable"); obj != nil {
		t.Fatalf("evo.ErrExecSpecMissingExecutable still exists; ErrExecPathMissing replaces it")
	}
}

// Executable became Path, map Env became os/exec []string, []string Outputs
// became typed File/Tree Outputs, and operation-level Basis moved to the Task.
func TestRemoved_ExecSpecFieldsOnExec(t *testing.T) {
	st := evoStruct(t, "Exec")
	for _, f := range fields(st) {
		if f.name == "Executable" || f.name == "Basis" {
			t.Errorf("evo.Exec has the 1.1 field %s %s", f.name, f.typ)
		}
		if f.name == "Env" && f.typ == "map[string]string" {
			t.Errorf("evo.Exec.Env is still the 1.1 merge map; ZYS-1382 uses os/exec []string")
		}
		if f.name == "Outputs" && f.typ == "[]string" {
			t.Errorf("evo.Exec.Outputs is still 1.1 []string paths; ZYS-1382 uses typed File/Tree Outputs")
		}
	}
}

func TestExecShape_FieldsAreExactlyTheProcessVocabulary(t *testing.T) {
	want := []field{
		{"Path", "string"},
		{"Args", "[]string"},
		{"Dir", "string"},
		{"Env", "[]string"},
		{"Outputs", "evo.Outputs"},
	}
	if got := fields(evoStruct(t, "Exec")); !slices.Equal(got, want) {
		t.Fatalf("evo.Exec fields = %v, want %v", got, want)
	}
}

func TestExecShape_RunIsAValueMethod(t *testing.T) {
	named := evoType(t, "Exec")
	sel := types.NewMethodSet(named).Lookup(named.Obj().Pkg(), "Run")
	if sel == nil {
		t.Fatal("evo.Exec (value receiver) has no Run method")
	}
	q := func(p *types.Package) string { return p.Name() }
	got := types.TypeString(sel.Type(), q)
	if want := "func(ctx context.Context) (evo.ExecResult, error)"; got != want {
		t.Fatalf("evo.Exec.Run = %s, want %s", got, want)
	}
}

func TestExecShape_ResultFields(t *testing.T) {
	want := []field{
		{"Ran", "bool"},
		{"ExitCode", "int"},
		{"Stdout", "string"},
		{"Stderr", "string"},
		{"Truncated", "bool"},
	}
	if got := fields(evoStruct(t, "ExecResult")); !slices.Equal(got, want) {
		t.Fatalf("evo.ExecResult fields = %v, want %v", got, want)
	}
}

// Outputs is a slice of a sealed element type: File and Tree satisfy it,
// callers cannot implement it, and there is no exported superclass.
func TestExecShape_OutputsIsASealedSliceOfFileAndTree(t *testing.T) {
	slice, ok := evoType(t, "Outputs").Underlying().(*types.Slice)
	if !ok {
		t.Fatalf("evo.Outputs is %s, want a slice", evoType(t, "Outputs").Underlying())
	}
	elem := slice.Elem()
	if named, ok := types.Unalias(elem).(*types.Named); ok && named.Obj().Exported() {
		t.Errorf("Outputs element %s is exported; ZYS-1382 forbids a public File/Tree superclass", named.Obj().Name())
	}
	iface, ok := elem.Underlying().(*types.Interface)
	if !ok {
		t.Fatalf("Outputs element is %s, want an interface", elem)
	}
	if iface.NumMethods() == 0 {
		t.Fatal("Outputs element is an empty interface; it must be sealed by an unexported method")
	}
	for m := range iface.Methods() {
		if m.Exported() {
			t.Errorf("Outputs element has exported method %s; callers could implement it", m.Name())
		}
	}
	for _, name := range []string{"File", "Tree"} {
		if !types.Implements(evoType(t, name), iface) {
			t.Errorf("evo.%s does not satisfy the Outputs element type", name)
		}
	}
}

func TestExecShape_ErrorsAreErrorVars(t *testing.T) {
	errorType := types.Universe.Lookup("error").Type()
	for _, name := range []string{
		"ErrExecPathMissing",
		"ErrExecExecutableNotFound",
		"ErrExecNonzeroExit",
		"ErrExecOutputMissingAfterSuccess",
	} {
		v, ok := evoScope(t).Lookup(name).(*types.Var)
		if !ok {
			t.Errorf("evo.%s is not a package variable", name)
			continue
		}
		if !types.Identical(v.Type(), errorType) {
			t.Errorf("evo.%s has type %s, want error", name, v.Type())
		}
	}
}
