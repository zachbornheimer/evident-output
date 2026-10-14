package evo_test

import (
	"go/importer"
	"go/token"
	"go/types"
	"os"
	"sync"
	"testing"
)

// evoModulePath is the import path of package evo.
const evoModulePath = "github.com/zachbornheimer/evident-output/v2"

// loadContractTypes type-checks package evo from source once per process, so
// contract tests can compare CONTRACT.md's types with the package's own.
var loadContractTypes = sync.OnceValues(func() (*types.Package, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	imp := importer.ForCompiler(token.NewFileSet(), "source", nil).(types.ImporterFrom)
	return imp.ImportFrom(evoModulePath, dir, 0)
})

func contractTypes(t *testing.T) *types.Package {
	t.Helper()
	pkg, err := loadContractTypes()
	if err != nil {
		t.Fatalf("type-check package evo: %v", err)
	}
	return pkg
}

// namedType is package evo's defined type name.
func namedType(t *testing.T, pkg *types.Package, name string) *types.Named {
	t.Helper()
	obj, ok := pkg.Scope().Lookup(name).(*types.TypeName)
	if !ok {
		t.Fatalf("package evo declares no type %s", name)
	}
	named, ok := obj.Type().(*types.Named)
	if !ok {
		t.Fatalf("evo.%s is %s, not a defined type", name, obj.Type())
	}
	return named
}

// method is the method name of *recv.
func method(t *testing.T, pkg *types.Package, recv, name string) *types.Signature {
	t.Helper()
	ptr := types.NewPointer(namedType(t, pkg, recv))
	obj, _, _ := types.LookupFieldOrMethod(ptr, true, pkg, name)
	fn, ok := obj.(*types.Func)
	if !ok {
		t.Fatalf("*evo.%s has no method %s", recv, name)
	}
	return fn.Type().(*types.Signature)
}

// TestAfterTakesSealedPredecessor pins CONTRACT.md §2: After takes
// ...Predecessor, a sealed interface that only *Task, *Group, *Sequence and
// *Computed[T] implement, so After("build") does not compile.
func TestAfterTakesSealedPredecessor(t *testing.T) {
	pkg := contractTypes(t)
	for _, recv := range []string{"Task", "Group", "Sequence"} {
		sig := method(t, pkg, recv, "After")
		params := sig.Params()
		if !sig.Variadic() || params.Len() != 1 {
			t.Errorf("(*%s).After has type %s, want After(preds ...Predecessor)", recv, sig)
			continue
		}
		elem := params.At(0).Type().(*types.Slice).Elem()
		named, ok := elem.(*types.Named)
		if !ok || named.Obj().Pkg() != pkg || named.Obj().Name() != "Predecessor" {
			t.Errorf("(*%s).After takes ...%s, want ...Predecessor", recv, elem)
		}
	}

	pred, ok := pkg.Scope().Lookup("Predecessor").(*types.TypeName)
	if !ok {
		t.Fatal("package evo declares no Predecessor type")
	}
	iface, ok := pred.Type().Underlying().(*types.Interface)
	if !ok {
		t.Fatalf("Predecessor is %s, want an interface", pred.Type().Underlying())
	}
	if !sealed(iface) {
		t.Errorf("Predecessor %s has no unexported method, so code outside evo can implement it", iface)
	}
	computedInt, err := types.Instantiate(nil, namedType(t, pkg, "Computed"), []types.Type{types.Typ[types.Int]}, true)
	if err != nil {
		t.Fatalf("instantiate Computed[int]: %v", err)
	}
	implementers := map[string]types.Type{
		"*Task":          types.NewPointer(namedType(t, pkg, "Task")),
		"*Group":         types.NewPointer(namedType(t, pkg, "Group")),
		"*Sequence":      types.NewPointer(namedType(t, pkg, "Sequence")),
		"*Computed[int]": types.NewPointer(computedInt),
	}
	for name, typ := range implementers {
		if !types.Implements(typ, iface) {
			t.Errorf("%s does not implement Predecessor", name)
		}
	}
}

// sealed reports whether iface has an unexported method, which only its
// own package can implement.
func sealed(iface *types.Interface) bool {
	for m := range iface.Methods() {
		if !m.Exported() {
			return true
		}
	}
	return false
}
