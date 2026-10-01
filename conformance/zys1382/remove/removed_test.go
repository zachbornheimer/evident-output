package remove_test

// Migration tests for the remove primitive. Nothing in the 1.1 surface is
// superseded by Remove (1.1 had no removal verb on File; Task.Delete and its
// siblings left in 1.1), so these tests pin that the new surface exists with
// exact kinds, and that no implicit-absence spelling crept in. They load
// package evo with go/types (source importer), so they compile today and
// fail until the surface is real.

import (
	"go/importer"
	"go/token"
	"go/types"
	"slices"
	"strings"
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
		t.Fatalf("load %s: %v", evoImportPath, err)
	}
	return pkg.Scope()
}

func structOf(t *testing.T, name string) (*types.Named, *types.Struct) {
	t.Helper()
	tn, ok := evoScope(t).Lookup(name).(*types.TypeName)
	if !ok {
		t.Fatalf("evo.%s must be a type name (struct), got %T", name, evoScope(t).Lookup(name))
	}
	named, ok := tn.Type().(*types.Named)
	if !ok {
		t.Fatalf("evo.%s must be a defined type, not an alias", name)
	}
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		t.Fatalf("evo.%s underlying type is %v, want a struct", name, named.Underlying())
	}
	return named, st
}

func fieldNames(st *types.Struct) []string {
	names := make([]string, st.NumFields())
	for i := range names {
		names[i] = st.Field(i).Name()
	}
	return names
}

func TestShape_RemoveIsAValueMethodOnFileAndTree(t *testing.T) {
	for _, name := range []string{"File", "Tree"} {
		named, _ := structOf(t, name)
		sel := types.NewMethodSet(named).Lookup(nil, "Remove")
		if sel == nil {
			t.Errorf("%s value method set has no Remove (missing or pointer receiver)", name)
			continue
		}
		sig := sel.Obj().(*types.Func).Type().(*types.Signature)
		if sig.Params().Len() != 1 || sig.Params().At(0).Type().String() != "context.Context" ||
			sig.Results().Len() != 1 || sig.Results().At(0).Type().String() != "error" {
			t.Errorf("%s.Remove = %s, want func(context.Context) error", name, sig)
		}
	}
}

// Absence is only ever asked for by the verb: no field, mode, or pointer
// receiver spelling may declare "make this absent".
func TestShape_NoImplicitAbsenceSpelling(t *testing.T) {
	fileFields := []string{"Path", "Content", "Mode"}
	treeFields := []string{"Path", "Content"}
	for name, want := range map[string][]string{"File": fileFields, "Tree": treeFields} {
		_, st := structOf(t, name)
		if got := fieldNames(st); !slices.Equal(got, want) {
			t.Errorf("%s fields = %v, want exactly %v (no Absent/Delete/Remove flag)", name, got, want)
		}
	}
	for _, name := range []string{"Remove", "Delete", "Absent", "Absence", "Rm", "RemoveAll"} {
		if obj := evoScope(t).Lookup(name); obj != nil {
			t.Errorf("evo.%s exists (%T); removal is only the File.Remove / Tree.Remove methods", name, obj)
		}
	}
}

// 1.1 retired the Task.Delete family; the removal verb must not return as
// a Task method or a Remove-prefixed content producer.
func TestRemoved_NoTaskDeleteFamilyOrRemoveContent(t *testing.T) {
	scope := evoScope(t)
	task, ok := scope.Lookup("TaskHandle").(*types.TypeName)
	if !ok {
		t.Fatalf("evo.TaskHandle must exist, got %T", scope.Lookup("TaskHandle"))
	}
	methods := types.NewMethodSet(types.NewPointer(task.Type()))
	for method := range methods.Methods() {
		name := method.Obj().Name()
		if strings.HasPrefix(name, "Delete") || strings.HasPrefix(name, "Remove") {
			t.Errorf("TaskHandle.%s exists; removal goes through File.Remove / Tree.Remove", name)
		}
	}
}

func TestShape_RemovalErrorsExistAsErrorVariables(t *testing.T) {
	for _, name := range []string{
		"ErrPathMissing", "ErrContentMissing", "ErrVerifyMismatch", "ErrNoTaskContext",
		"ErrFilePathTypeMismatch", "ErrTreePathTypeMismatch",
	} {
		v, ok := evoScope(t).Lookup(name).(*types.Var)
		if !ok {
			t.Errorf("evo.%s must exist as an exported error variable", name)
			continue
		}
		if v.Type().String() != "error" {
			t.Errorf("evo.%s has type %v, want error", name, v.Type())
		}
	}
}
