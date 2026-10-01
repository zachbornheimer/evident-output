package tree_test

// Removal and shape tests for the Tree primitive. They load package evo from
// source with go/types, so they compile on any state of the package and
// assert on its scope: absent identifiers must be absent, new ones must have
// exactly the expected kinds. Nothing here imports package evo directly.

import (
	"go/importer"
	"go/token"
	"go/types"
	"os"
	"slices"
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

func lookupType(t *testing.T, name string) *types.Named {
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
		t.Fatalf("evo.%s is %v, want a defined type, not an alias of another type", name, tn.Type())
	}
	return named
}

func structFields(t *testing.T, named *types.Named) []string {
	t.Helper()
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		t.Fatalf("evo.%s underlying type is %v, want a plain struct", named.Obj().Name(), named.Underlying())
	}
	fields := make([]string, st.NumFields())
	for i := range fields {
		f := st.Field(i)
		fields[i] = f.Name() + " " + types.TypeString(f.Type(), qualifier)
	}
	return fields
}

// TestRemoved_FSPathIsNoLongerAPublicIdentifier: FSPath was the only tree
// fingerprint in 1.1; File and Tree are the filesystem identity now.
func TestRemoved_FSPathIsNoLongerAPublicIdentifier(t *testing.T) {
	if obj := evoPackage(t).Scope().Lookup("FSPath"); obj != nil {
		t.Fatalf("evo.FSPath still exists (%v); Tree replaces it as the tree identity", obj)
	}
}

// TestRemoved_NoPublicFilesystemSuperclass: File and Tree are siblings with a
// shared vocabulary, never a named common parent.
func TestRemoved_NoPublicFilesystemSuperclass(t *testing.T) {
	for _, name := range []string{"State", "Artifact", "FSObject", "FSState"} {
		if obj := evoPackage(t).Scope().Lookup(name); obj != nil {
			t.Errorf("evo.%s exists (%v); the File/Tree union must stay unexported", name, obj)
		}
	}
}

func TestTreeIsAPlainStructWithExactFields(t *testing.T) {
	got := structFields(t, lookupType(t, "Tree"))
	want := []string{"Path string", "Content evo.TreeContent"}
	if !slices.Equal(got, want) {
		t.Fatalf("evo.Tree fields = %v, want %v", got, want)
	}
}

func TestExtractIsAPlainStructWithExactFields(t *testing.T) {
	got := structFields(t, lookupType(t, "Extract"))
	want := []string{"File evo.File", "Root string"}
	if !slices.Equal(got, want) {
		t.Fatalf("evo.Extract fields = %v, want %v", got, want)
	}
}

// TestTreeContentIsSealed: callers can hold a TreeContent but cannot
// implement one, so the interface has only unexported methods.
func TestTreeContentIsSealed(t *testing.T) {
	iface, ok := lookupType(t, "TreeContent").Underlying().(*types.Interface)
	if !ok {
		t.Fatalf("evo.TreeContent is not an interface")
	}
	if iface.NumMethods() == 0 {
		t.Fatalf("evo.TreeContent has no methods; it must carry an unexported marker so it is sealed")
	}
	for m := range iface.Methods() {
		if m.Exported() {
			t.Errorf("evo.TreeContent has exported method %s; it must be sealed", m.Name())
		}
	}
}

func TestExtractSatisfiesTreeContentAndFileDoesNot(t *testing.T) {
	pkg := evoPackage(t)
	content := lookupType(t, "TreeContent").Underlying().(*types.Interface)
	if !types.Implements(lookupType(t, "Extract"), content) {
		t.Errorf("evo.Extract does not satisfy evo.TreeContent")
	}
	fileContent := pkg.Scope().Lookup("Download")
	if fileContent != nil && types.Implements(fileContent.Type(), content) {
		t.Errorf("evo.Download satisfies evo.TreeContent; File and Tree content producers must stay distinct")
	}
}

// TestTreeMethodSetIsExactlyTheSharedVocabulary pins every method's name,
// receiver kind (value), and signature. Extra exported methods are a
// vocabulary leak.
func TestTreeMethodSetIsExactlyTheSharedVocabulary(t *testing.T) {
	tree := lookupType(t, "Tree")
	want := map[string]string{
		"Read":     "(context.Context) ([]evo.File, error)",
		"Write":    "(context.Context) error",
		"Verify":   "(context.Context) error",
		"Equal":    "(context.Context, evo.Tree, ...evo.ChecksumOption) (bool, error)",
		"Remove":   "(context.Context) error",
		"Checksum": "(context.Context, ...evo.ChecksumOption) (string, error)",
	}
	valueSet := types.NewMethodSet(tree)
	got := map[string]string{}
	for sel := range valueSet.Methods() {
		fn := sel.Obj().(*types.Func)
		if !fn.Exported() {
			continue
		}
		sig := fn.Type().(*types.Signature)
		got[fn.Name()] = signatureShape(sig)
	}
	for name, shape := range want {
		g, ok := got[name]
		if !ok {
			t.Errorf("evo.Tree has no value-receiver method %s (pointer-receiver methods do not count: literals must call them)", name)
			continue
		}
		if g != shape {
			t.Errorf("evo.Tree.%s = %s, want %s", name, g, shape)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("evo.Tree has unexpected exported method %s", name)
		}
	}
}

// signatureShape prints parameter and result types without names.
func signatureShape(sig *types.Signature) string {
	params := make([]string, sig.Params().Len())
	for i := range params {
		typ := types.TypeString(sig.Params().At(i).Type(), qualifier)
		if sig.Variadic() && i == len(params)-1 {
			typ = "..." + strings.TrimPrefix(typ, "[]")
		}
		params[i] = typ
	}
	results := make([]string, sig.Results().Len())
	for i := range results {
		results[i] = types.TypeString(sig.Results().At(i).Type(), qualifier)
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

func TestExcludeIsAFunctionTakingAPatternReturningAChecksumOption(t *testing.T) {
	obj := evoPackage(t).Scope().Lookup("Exclude")
	if obj == nil {
		t.Fatalf("evo.Exclude does not exist")
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		t.Fatalf("evo.Exclude is a %T, want a function", obj)
	}
	if got, want := signatureShape(fn.Type().(*types.Signature)), "(string) evo.ChecksumOption"; got != want {
		t.Fatalf("evo.Exclude = %s, want %s", got, want)
	}
}

func TestTreeErrorsExistAsErrorValues(t *testing.T) {
	scope := evoPackage(t).Scope()
	errorType := types.Universe.Lookup("error").Type()
	for _, name := range []string{
		"ErrPathMissing", "ErrContentMissing", "ErrVerifyMismatch",
		"ErrTreePathTypeMismatch", "ErrNoTaskContext", "ErrExtractMalformed", "ErrExtractUnsafeEntry",
	} {
		obj, ok := scope.Lookup(name).(*types.Var)
		if !ok {
			t.Errorf("evo.%s is not a package-level variable", name)
			continue
		}
		if !types.Identical(obj.Type(), errorType) {
			t.Errorf("evo.%s has type %v, want error", name, obj.Type())
		}
	}
}
