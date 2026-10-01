package patch_test

// Migration tests for the patch primitive. They load package evo with
// go/types (source importer) instead of referencing symbols, so they compile
// today and fail until the 1.1 Patch -> FileSet -> Files ceremony is gone.

import (
	"go/importer"
	"go/token"
	"go/types"
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

func TestRemoved_FileSetTypeIsGone(t *testing.T) {
	if obj := evoScope(t).Lookup("FileSet"); obj != nil {
		t.Fatalf("evo.FileSet still exists (%T); Patch applies directly, with no derived set to commit", obj)
	}
}

func TestRemoved_FilesFunctionIsGone(t *testing.T) {
	if obj := evoScope(t).Lookup("Files"); obj != nil {
		t.Fatalf("evo.Files still exists (%T); Patch commits through File itself", obj)
	}
}

// ErrStaleBasis was Files' stale-derivation error; a concurrent edit under
// Patch is ErrPatchStale. Delete and rename are standard forms now, so their
// Unsupported errors describe behavior that no longer exists.
func TestRemoved_CeremonyErrorsAreGone(t *testing.T) {
	gone := map[string]string{
		"ErrStaleBasis":             "ErrPatchStale",
		"ErrPatchDeleteUnsupported": "delete is a supported form",
		"ErrPatchRenameUnsupported": "rename is a supported form",
	}
	for name, why := range gone {
		if obj := evoScope(t).Lookup(name); obj != nil {
			t.Errorf("evo.%s still exists (%T); replaced by: %s", name, obj, why)
		}
	}
}

func TestShape_PatchIsAFunctionReturningOnlyError(t *testing.T) {
	fn, ok := evoScope(t).Lookup("Patch").(*types.Func)
	if !ok {
		t.Fatalf("evo.Patch is %T, want a function", evoScope(t).Lookup("Patch"))
	}
	sig := fn.Type().(*types.Signature)
	if sig.Recv() != nil || sig.TypeParams().Len() != 0 || sig.Variadic() {
		t.Fatalf("evo.Patch = %v, want plain func(context.Context, []byte) error", sig)
	}
	params, results := sig.Params(), sig.Results()
	if params.Len() != 2 || results.Len() != 1 ||
		types.TypeString(params.At(0).Type(), nil) != "context.Context" ||
		types.TypeString(params.At(1).Type(), nil) != "[]byte" ||
		types.TypeString(results.At(0).Type(), nil) != "error" {
		t.Fatalf("evo.Patch = %v, want func(context.Context, []byte) error", sig)
	}
}

func TestShape_PatchErrorsExist(t *testing.T) {
	errorType := types.Universe.Lookup("error").Type()
	for _, name := range []string{
		"ErrPatchMalformed", "ErrPatchDoesNotApply", "ErrPatchUnsupported",
		"ErrPatchUnsafePath", "ErrPatchStale", "ErrNoTaskContext",
	} {
		v, ok := evoScope(t).Lookup(name).(*types.Var)
		if !ok {
			t.Errorf("evo.%s must exist as an exported error variable, got %T", name, evoScope(t).Lookup(name))
			continue
		}
		if !types.Implements(v.Type(), errorType.Underlying().(*types.Interface)) {
			t.Errorf("evo.%s has type %v, want an error", name, v.Type())
		}
	}
}
