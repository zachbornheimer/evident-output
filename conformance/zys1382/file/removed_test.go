package file_test

// Migration tests for the file primitive. They load package evo with go/types
// (source importer) instead of referencing symbols, so they compile today and
// fail until the 1.1 FileSpec / evo.File(ctx, spec) shape is really gone.

import (
	"fmt"
	"go/importer"
	"go/token"
	"go/types"
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

func evoFileType(t *testing.T) (*types.Named, *types.Struct) {
	t.Helper()
	obj := evoScope(t).Lookup("File")
	tn, ok := obj.(*types.TypeName)
	if !ok {
		t.Fatalf("evo.File is %T, want a type name (struct)", obj)
	}
	named, ok := tn.Type().(*types.Named)
	if !ok {
		t.Fatalf("evo.File is %v, want a defined struct type, not an alias", tn.Type())
	}
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		t.Fatalf("evo.File underlying type is %v, want a struct", named.Underlying())
	}
	return named, st
}

// typeName renders a type with import-path qualifiers and byte as byte.
func typeName(typ types.Type) string {
	s := types.TypeString(typ, func(p *types.Package) string { return p.Path() })
	return strings.ReplaceAll(s, "uint8", "byte")
}

func tupleNames(tup *types.Tuple) string {
	names := make([]string, tup.Len())
	for i := range names {
		names[i] = typeName(tup.At(i).Type())
	}
	return strings.Join(names, ",")
}

func signature(sig *types.Signature) string {
	return fmt.Sprintf("(%s)->(%s)", tupleNames(sig.Params()), tupleNames(sig.Results()))
}

func TestRemoved_FileSpecTypeIsGone(t *testing.T) {
	if obj := evoScope(t).Lookup("FileSpec"); obj != nil {
		t.Fatalf("evo.FileSpec still exists (%T); File is the plain struct now", obj)
	}
}

func TestRemoved_FileFunctionFormIsGone(t *testing.T) {
	obj := evoScope(t).Lookup("File")
	if _, isFunc := obj.(*types.Func); isFunc {
		t.Fatalf("evo.File is still the function form evo.File(ctx, spec); it must be the File struct type")
	}
	if _, isType := obj.(*types.TypeName); !isType {
		t.Fatalf("evo.File is %T, want a type name", obj)
	}
}

// Both errors belonged to FileSpec: the missing-path error is now the shared
// ErrPathMissing and the unmanaged-contents error is now ErrContentMissing.
func TestRemoved_FileSpecErrorsAreGone(t *testing.T) {
	for _, name := range []string{"ErrFileSpecMissingPath", "ErrFileUnmanagedContentsMissing"} {
		if obj := evoScope(t).Lookup(name); obj != nil {
			t.Errorf("evo.%s still exists; replaced by ErrPathMissing / ErrContentMissing", name)
		}
	}
	for _, name := range []string{"ErrPathMissing", "ErrContentMissing", "ErrVerifyMismatch"} {
		if _, ok := evoScope(t).Lookup(name).(*types.Var); !ok {
			t.Errorf("evo.%s must exist as an exported error variable", name)
		}
	}
}

func TestShape_FileIsAPlainStructWithExactFields(t *testing.T) {
	_, st := evoFileType(t)
	want := []struct{ name, typ string }{
		{"Path", "string"},
		{"Content", evoImportPath + ".FileContent"},
		{"Mode", "io/fs.FileMode"},
	}
	if st.NumFields() != len(want) {
		t.Fatalf("File has %d fields, want exactly %d (Path, Content, Mode)", st.NumFields(), len(want))
	}
	for i, w := range want {
		f := st.Field(i)
		if f.Name() != w.name || typeName(f.Type()) != w.typ || !f.Exported() || f.Embedded() {
			t.Errorf("field %d = %s %s (exported=%v embedded=%v), want %s %s", i, f.Name(), typeName(f.Type()), f.Exported(), f.Embedded(), w.name, w.typ)
		}
	}
}

// Value receivers are part of the contract: evo.File{...}.Write(ctx) must
// compile on a non-addressable literal.
func TestShape_FileMethodsAreValueReceiversWithExactSignatures(t *testing.T) {
	named, _ := evoFileType(t)
	file := evoImportPath + ".File"
	want := map[string]string{
		"Read":     "(context.Context)->([]byte,error)",
		"Write":    "(context.Context)->(error)",
		"Verify":   "(context.Context)->(error)",
		"Equal":    "(context.Context," + file + ")->(bool,error)",
		"Remove":   "(context.Context)->(error)",
		"Checksum": "(context.Context)->(string,error)",
	}
	valueMethods := types.NewMethodSet(named)
	for name, sig := range want {
		sel := valueMethods.Lookup(nil, name)
		if sel == nil {
			t.Errorf("File value method set has no %s (pointer receiver, or missing)", name)
			continue
		}
		fn := sel.Obj().(*types.Func)
		if got := signature(fn.Type().(*types.Signature)); got != sig {
			t.Errorf("File.%s = %s, want %s", name, got, sig)
		}
	}
}

func TestShape_FileContentIsSealedAndBytesAndDownloadSatisfyIt(t *testing.T) {
	scope := evoScope(t)
	contentObj, ok := scope.Lookup("FileContent").(*types.TypeName)
	if !ok {
		t.Fatalf("evo.FileContent must exist as a type name, got %T", scope.Lookup("FileContent"))
	}
	iface, ok := contentObj.Type().Underlying().(*types.Interface)
	if !ok {
		t.Fatalf("evo.FileContent is %v, want an interface", contentObj.Type().Underlying())
	}
	if iface.NumMethods() == 0 {
		t.Fatalf("evo.FileContent has no methods; it must be sealed by an unexported method")
	}
	for m := range iface.Methods() {
		if m.Exported() {
			t.Errorf("evo.FileContent has exported method %s; callers must not be able to implement it", m.Name())
		}
	}

	bytesFn, ok := scope.Lookup("Bytes").(*types.Func)
	if !ok {
		t.Fatalf("evo.Bytes must exist as a function, got %T", scope.Lookup("Bytes"))
	}
	sig := bytesFn.Type().(*types.Signature)
	if sig.TypeParams().Len() != 1 || sig.Params().Len() != 1 || sig.Results().Len() != 1 ||
		typeName(sig.Results().At(0).Type()) != evoImportPath+".FileContent" {
		t.Errorf("evo.Bytes = %s, want generic func Bytes[T ~string | ~[]byte](v T) FileContent", sig)
	}

	download, ok := scope.Lookup("Download").(*types.TypeName)
	if !ok {
		t.Fatalf("evo.Download must exist as a type name, got %T", scope.Lookup("Download"))
	}
	if !types.Implements(download.Type(), iface) {
		t.Errorf("evo.Download (value) does not satisfy evo.FileContent")
	}
}

// File and Tree are the same concept at two cardinalities; no public
// superclass may unify them.
func TestShape_NoPublicFilesystemSuperclass(t *testing.T) {
	for _, name := range []string{"State", "Artifact", "FSObject"} {
		if obj := evoScope(t).Lookup(name); obj != nil {
			t.Errorf("evo.%s exists (%T); the contract has no public superclass over File and Tree", name, obj)
		}
	}
}
