package checksum_test

// Removal and shape tests for Checksum. They type-check package evo from
// source with go/types and assert on its scope: superseded identifiers must
// be absent, and the new ones must have exactly the decided shape. Nothing in
// this file imports evo, so absence is a test failure, never a compile error
// here (the sibling files of this package do import the new surface).

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
	evoOnce   sync.Once
	evoPkg    *types.Package
	evoPkgErr error
)

// evoPackage type-checks package evo from source once per test binary.
func evoPackage(t *testing.T) *types.Package {
	t.Helper()
	evoOnce.Do(func() {
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

// shapeQualifier prints types the way a caller spells them: evo.File.
func shapeQualifier(p *types.Package) string { return p.Name() }

func lookupNamed(t *testing.T, name string) *types.Named {
	t.Helper()
	obj := evoPackage(t).Scope().Lookup(name)
	if obj == nil {
		t.Fatalf("evo.%s does not exist", name)
	}
	tn, ok := obj.(*types.TypeName)
	if !ok {
		t.Fatalf("evo.%s is a %T, want a type", name, obj)
	}
	named, ok := tn.Type().(*types.Named)
	if !ok {
		t.Fatalf("evo.%s is %v, want a defined type, not an alias", name, tn.Type())
	}
	return named
}

// shapeOf prints a signature's parameter and result types without names.
func shapeOf(sig *types.Signature) string {
	params := make([]string, sig.Params().Len())
	for i := range params {
		typ := types.TypeString(sig.Params().At(i).Type(), shapeQualifier)
		if sig.Variadic() && i == len(params)-1 {
			typ = "..." + strings.TrimPrefix(typ, "[]")
		}
		params[i] = typ
	}
	results := make([]string, sig.Results().Len())
	for i := range results {
		results[i] = types.TypeString(sig.Results().At(i).Type(), shapeQualifier)
	}
	out := "(" + strings.Join(params, ", ") + ")"
	switch len(results) {
	case 0:
	case 1:
		out += " " + results[0]
	default:
		out += " (" + strings.Join(results, ", ") + ")"
	}
	return out
}

// valueMethod returns the value-receiver method name of typ, or nil.
// Pointer-receiver methods do not count: literals must be able to call it.
func valueMethod(typ *types.Named, name string) *types.Func {
	sel := types.NewMethodSet(typ).Lookup(nil, name)
	if sel == nil {
		return nil
	}
	return sel.Obj().(*types.Func)
}

// FSPath was 1.1's only tree fingerprint, with its own Merkle algorithm.
// The checksum engine behind File.Checksum and Tree.Checksum replaces it.
func TestRemoved_FSPathFingerprintIsGone(t *testing.T) {
	if obj := evoPackage(t).Scope().Lookup("FSPath"); obj != nil {
		t.Fatalf("evo.FSPath still exists (%v); File/Tree Basis inputs replace it and share the one checksum engine", obj)
	}
}

func TestChecksumMethodsHaveTheDecidedShape(t *testing.T) {
	cases := []struct{ owner, want string }{
		{"File", "(context.Context) (string, error)"},
		{"Tree", "(context.Context, ...evo.ChecksumOption) (string, error)"},
	}
	for _, tc := range cases {
		fn := valueMethod(lookupNamed(t, tc.owner), "Checksum")
		if fn == nil {
			t.Errorf("evo.%s has no value-receiver Checksum method", tc.owner)
			continue
		}
		if got := shapeOf(fn.Type().(*types.Signature)); got != tc.want {
			t.Errorf("evo.%s.Checksum = %s, want %s", tc.owner, got, tc.want)
		}
	}
}

func TestExcludeReturnsAChecksumOption(t *testing.T) {
	fn, ok := evoPackage(t).Scope().Lookup("Exclude").(*types.Func)
	if !ok {
		t.Fatalf("evo.Exclude is not a function")
	}
	if got, want := shapeOf(fn.Type().(*types.Signature)), "(string) evo.ChecksumOption"; got != want {
		t.Fatalf("evo.Exclude = %s, want %s", got, want)
	}
}

// ChecksumOption is opaque: callers obtain one only from Exclude (and future
// constructors), never by conversion, field access, or implementation.
func TestChecksumOptionIsOpaque(t *testing.T) {
	opt := lookupNamed(t, "ChecksumOption")
	switch u := opt.Underlying().(type) {
	case *types.Basic:
		t.Errorf("evo.ChecksumOption is a %v; a caller could convert any value into an option", u)
	case *types.Struct:
		for f := range u.Fields() {
			if f.Exported() {
				t.Errorf("evo.ChecksumOption has exported field %s", f.Name())
			}
		}
	case *types.Interface:
		for m := range u.Methods() {
			if m.Exported() {
				t.Errorf("evo.ChecksumOption has exported method %s; callers could implement it", m.Name())
			}
		}
	}
	for m := range opt.Methods() {
		if m.Exported() {
			t.Errorf("evo.ChecksumOption has exported method %s; it is an opaque value", m.Name())
		}
	}
}

// Each struct owns its Checksum; no public interface abstracts the shared
// vocabulary over File and Tree (no "Checksummer", "State", "FSObject").
func TestNoPublicInterfaceAbstractsFileAndTree(t *testing.T) {
	file, tree := lookupNamed(t, "File"), lookupNamed(t, "Tree")
	scope := evoPackage(t).Scope()
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok || !tn.Exported() {
			continue
		}
		iface, ok := tn.Type().Underlying().(*types.Interface)
		if !ok {
			continue
		}
		if iface.NumMethods() > 0 && hasExportedMethod(iface) && types.Implements(file, iface) && types.Implements(tree, iface) {
			t.Errorf("evo.%s is a public interface satisfied by both File and Tree; the union must stay sealed and unexported", name)
		}
		for method := range iface.Methods() {
			if method.Name() == "Checksum" {
				t.Errorf("evo.%s exposes Checksum as an interface method; File and Tree each own Checksum", name)
			}
		}
	}
}

// No named superclass survives under any spelling the 1.1 surface or an
// implementer might reach for.
func TestRemoved_NoPublicFilesystemSuperclass(t *testing.T) {
	scope := evoPackage(t).Scope()
	for _, name := range []string{"FSObject", "FSState", "FSEntry", "FSNode", "Checksummer", "Fingerprinter"} {
		if obj := scope.Lookup(name); obj != nil {
			t.Errorf("evo.%s exists (%v); File and Tree are plain structs with no public superclass", name, obj)
		}
	}
}

// The engine stays internal: no exported package-level function computes a
// checksum, so every caller goes through File.Checksum or Tree.Checksum.
func TestNoPackageLevelChecksumFunction(t *testing.T) {
	scope := evoPackage(t).Scope()
	for _, name := range scope.Names() {
		fn, ok := scope.Lookup(name).(*types.Func)
		if !ok || !fn.Exported() {
			continue
		}
		lower := strings.ToLower(name)
		if strings.Contains(lower, "checksum") || strings.Contains(lower, "digest") {
			t.Errorf("evo.%s %s is a package-level checksum entry point; the engine must be reachable only through File.Checksum and Tree.Checksum",
				name, shapeOf(fn.Type().(*types.Signature)))
		}
	}
}

func hasExportedMethod(iface *types.Interface) bool {
	for method := range iface.Methods() {
		if method.Exported() {
			return true
		}
	}
	return false
}
