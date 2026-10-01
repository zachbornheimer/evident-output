package basis_test

// Removal and shape tests for Task Basis. They load package evo from source
// with go/types, so they compile on any state of the package and assert on
// its scope: the 1.1 operation-level Basis (FileSpec.Basis, ExecSpec.Basis,
// FSPath) must be gone, and Basis must live on the Task with a sealed input
// type that File, Tree, Value, and App satisfy. Nothing here imports package
// evo directly.

import (
	"go/importer"
	"go/token"
	"go/types"
	"os"
	"strings"
	"sync"
	"testing"
)

const evoImportPath = "github.com/zachbornheimer/evident-output"

var (
	evoScopeOnce sync.Once
	evoPkg       *types.Package
	evoPkgErr    error
)

// evoPackage type-checks package evo from source once per test binary.
func evoPackage(t *testing.T) *types.Package {
	t.Helper()
	evoScopeOnce.Do(func() {
		cwd, err := os.Getwd()
		if err != nil {
			evoPkgErr = err
			return
		}
		imp := importer.ForCompiler(token.NewFileSet(), "source", nil).(types.ImporterFrom)
		evoPkg, evoPkgErr = imp.ImportFrom(evoImportPath, cwd, 0)
	})
	if evoPkgErr != nil {
		t.Fatalf("load package evo from source: %v", evoPkgErr)
	}
	return evoPkg
}

// qualifier prints types the way a caller spells them: evo.File, context.Context.
func qualifier(p *types.Package) string { return p.Name() }

func lookupObject(t *testing.T, name string) types.Object {
	t.Helper()
	obj := evoPackage(t).Scope().Lookup(name)
	if obj == nil {
		t.Fatalf("evo.%s does not exist", name)
	}
	return obj
}

func lookupTypeName(t *testing.T, name string) *types.TypeName {
	t.Helper()
	tn, ok := lookupObject(t, name).(*types.TypeName)
	if !ok {
		t.Fatalf("evo.%s is not a type", name)
	}
	return tn
}

// TestRemoved_OperationSpecsAreGone: FileSpec and ExecSpec carried the 1.1
// operation-level Basis. File and Exec are plain structs now and Basis
// moved to the Task, so neither spec type may survive under any form
// (type, alias, or variable).
func TestRemoved_OperationSpecsAreGone(t *testing.T) {
	for _, name := range []string{"FileSpec", "ExecSpec"} {
		if obj := evoPackage(t).Scope().Lookup(name); obj != nil {
			t.Errorf("evo.%s still exists (%v); its Basis field moved to task.Basis", name, obj)
		}
	}
}

// TestRemoved_FSPathIsGone: FSPath was the 1.1 filesystem fingerprint for a
// Basis. File and Tree are the filesystem identity of a Basis now.
func TestRemoved_FSPathIsGone(t *testing.T) {
	if obj := evoPackage(t).Scope().Lookup("FSPath"); obj != nil {
		t.Fatalf("evo.FSPath still exists (%v); pass evo.File or evo.Tree to task.Basis", obj)
	}
}

// TestRemoved_NoExportedStructCarriesABasisField: Basis is Task freshness,
// never a per-operation field. Every exported struct type (defined or
// aliased) is checked, so a Basis field reappearing on File, Tree, Exec, or
// any other struct fails here.
func TestRemoved_NoExportedStructCarriesABasisField(t *testing.T) {
	scope := evoPackage(t).Scope()
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok || !tn.Exported() {
			continue
		}
		st, ok := tn.Type().Underlying().(*types.Struct)
		if !ok {
			continue
		}
		for f := range st.Fields() {
			if f.Name() == "Basis" {
				t.Errorf("evo.%s has a field Basis %v; Basis belongs to the Task (task.Basis)", name, types.TypeString(f.Type(), qualifier))
			}
		}
	}
}

// basisMethod returns *TaskHandle's Basis method, failing when it is absent.
func basisMethod(t *testing.T) *types.Func {
	t.Helper()
	handle := lookupTypeName(t, "TaskHandle").Type()
	obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(handle), true, evoPackage(t), "Basis")
	fn, ok := obj.(*types.Func)
	if !ok {
		t.Fatalf("*evo.TaskHandle has no method Basis; Basis lives on the Task")
	}
	return fn
}

// basisInputType returns the element type of Basis's variadic parameter.
func basisInputType(t *testing.T) types.Type {
	t.Helper()
	sig := basisMethod(t).Type().(*types.Signature)
	if !sig.Variadic() || sig.Params().Len() != 1 {
		t.Fatalf("TaskHandle.Basis params = %v, want exactly one variadic parameter", sig.Params())
	}
	slice, ok := sig.Params().At(0).Type().(*types.Slice)
	if !ok {
		t.Fatalf("TaskHandle.Basis variadic parameter is %v, want a slice", sig.Params().At(0).Type())
	}
	return slice.Elem()
}

// TestBasisIsAChainableTaskMethod pins the result: Basis returns the same
// *TaskHandle so it chains before Define, like Key and After.
func TestBasisIsAChainableTaskMethod(t *testing.T) {
	sig := basisMethod(t).Type().(*types.Signature)
	if sig.Results().Len() != 1 {
		t.Fatalf("TaskHandle.Basis results = %v, want exactly *evo.TaskHandle", sig.Results())
	}
	if got := types.TypeString(sig.Results().At(0).Type(), qualifier); got != "*evo.TaskHandle" {
		t.Fatalf("TaskHandle.Basis returns %s, want *evo.TaskHandle", got)
	}
}

// TestBasisInputIsSealedAndUnexported: callers pass File, Tree, Value, or
// App, but cannot name or implement the input type. A public name for it
// would be the forbidden File/Tree superclass.
func TestBasisInputIsSealedAndUnexported(t *testing.T) {
	elem := basisInputType(t)
	named, ok := elem.(*types.Named)
	if !ok {
		t.Fatalf("TaskHandle.Basis input type is %v, want a named sealed interface (not any)", elem)
	}
	if named.Obj().Exported() {
		t.Errorf("TaskHandle.Basis input type evo.%s is exported; it must stay unexported", named.Obj().Name())
	}
	iface, ok := named.Underlying().(*types.Interface)
	if !ok {
		t.Fatalf("TaskHandle.Basis input type %s is not an interface", named.Obj().Name())
	}
	if iface.NumMethods() == 0 {
		t.Fatalf("TaskHandle.Basis input type %s has no methods; any value would satisfy it, so it is not sealed", named.Obj().Name())
	}
	sealed := false
	for method := range iface.Methods() {
		if !method.Exported() {
			sealed = true
		}
	}
	if !sealed {
		t.Errorf("TaskHandle.Basis input type %s has only exported methods; callers could implement it", named.Obj().Name())
	}
}

// TestFileAndTreeAreBasisInputs: File and Tree satisfy Basis as plain
// values (value receivers), so evo.File{Path: p} literals pass directly.
func TestFileAndTreeAreBasisInputs(t *testing.T) {
	elem := basisInputType(t)
	for _, name := range []string{"File", "Tree"} {
		typ := lookupTypeName(t, name).Type()
		if _, ok := typ.Underlying().(*types.Struct); !ok {
			t.Errorf("evo.%s is %v, want a plain struct", name, typ.Underlying())
			continue
		}
		if !types.AssignableTo(typ, elem) {
			t.Errorf("an evo.%s value is not assignable to TaskHandle.Basis's input type", name)
		}
	}
}

// TestValueAndAppRemainBasisInputs: the existing non-filesystem fingerprints
// stay valid Basis inputs with their 1.1 call shapes.
func TestValueAndAppRemainBasisInputs(t *testing.T) {
	elem := basisInputType(t)
	want := map[string]string{
		"Value": "(string, any)",
		"App":   "()",
	}
	for name, params := range want {
		fn, ok := lookupObject(t, name).(*types.Func)
		if !ok {
			t.Errorf("evo.%s is not a function", name)
			continue
		}
		sig := fn.Type().(*types.Signature)
		if got := paramShape(sig); got != params {
			t.Errorf("evo.%s params = %s, want %s", name, got, params)
		}
		if sig.Results().Len() != 1 {
			t.Errorf("evo.%s returns %v, want one Basis input", name, sig.Results())
			continue
		}
		if res := sig.Results().At(0).Type(); !types.AssignableTo(res, elem) {
			t.Errorf("evo.%s returns %s, which is not a TaskHandle.Basis input", name, types.TypeString(res, qualifier))
		}
	}
}

// TestBasisAcceptsOnlyIdentities: Basis inputs are File, Tree, Value, and
// App. A bare path string, a content producer, or a process is not an
// identity and must not type-check as one. Types that do not exist yet are
// skipped here; their own primitive's removed_test pins their existence.
func TestBasisAcceptsOnlyIdentities(t *testing.T) {
	elem := basisInputType(t)
	if types.AssignableTo(types.Typ[types.String], elem) {
		t.Errorf("a string is assignable to TaskHandle.Basis's input type; a path must be named as evo.File or evo.Tree")
	}
	for _, name := range []string{"Exec", "Download", "Extract", "FileContent", "TreeContent", "TaskHandle"} {
		tn, ok := evoPackage(t).Scope().Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		for _, typ := range []types.Type{tn.Type(), types.NewPointer(tn.Type())} {
			if types.AssignableTo(typ, elem) {
				t.Errorf("%s is assignable to TaskHandle.Basis's input type; only File, Tree, Value, and App are Basis inputs", types.TypeString(typ, qualifier))
			}
		}
	}
}

// TestBasisAfterDefineIsAnErrorValue: the misuse error for a Basis call
// after Define is a package-level error variable.
func TestBasisAfterDefineIsAnErrorValue(t *testing.T) {
	v, ok := lookupObject(t, "ErrBasisAfterDefine").(*types.Var)
	if !ok {
		t.Fatalf("evo.ErrBasisAfterDefine is not a package-level variable")
	}
	if !types.Identical(v.Type(), types.Universe.Lookup("error").Type()) {
		t.Fatalf("evo.ErrBasisAfterDefine has type %v, want error", v.Type())
	}
}

// paramShape prints parameter types without names.
func paramShape(sig *types.Signature) string {
	params := make([]string, sig.Params().Len())
	for i := range params {
		typ := types.TypeString(sig.Params().At(i).Type(), qualifier)
		if sig.Variadic() && i == len(params)-1 {
			typ = "..." + strings.TrimPrefix(typ, "[]")
		}
		params[i] = typ
	}
	return "(" + strings.Join(params, ", ") + ")"
}
