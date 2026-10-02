package download_test

// Migration tests for the Download primitive. Download itself is new in
// ZYS-1382, so what is "removed" is the 1.1 file shape it replaces: the
// FileSpec struct, its Contents and Basis fields, and the File function (the
// identifier File becomes a type). These load package evo from source and
// inspect its scope, so they compile today and fail until the migration lands.

import (
	"go/importer"
	"go/token"
	"go/types"
	"os"
	"strings"
	"sync"
	"testing"
)

const evoPath = "github.com/zachbornheimer/evident-output"

var (
	loadOnce sync.Once
	evoPkg   *types.Package
	loadErr  error
)

func loadEvo(t *testing.T) *types.Scope {
	t.Helper()
	loadOnce.Do(func() {
		wd, err := os.Getwd()
		if err != nil {
			loadErr = err
			return
		}
		imp := importer.ForCompiler(token.NewFileSet(), "source", nil).(types.ImporterFrom)
		evoPkg, loadErr = imp.ImportFrom(evoPath, wd, 0)
	})
	if loadErr != nil {
		t.Fatalf("loading package evo from source: %v", loadErr)
	}
	return evoPkg.Scope()
}

func typeNamed(t *testing.T, name string) (*types.TypeName, *types.Struct) {
	t.Helper()
	obj := loadEvo(t).Lookup(name)
	if obj == nil {
		t.Fatalf("evo.%s does not exist", name)
	}
	tn, ok := obj.(*types.TypeName)
	if !ok {
		t.Fatalf("evo.%s is a %T, want a type", name, obj)
	}
	st, ok := tn.Type().Underlying().(*types.Struct)
	if !ok {
		t.Fatalf("evo.%s underlies %s, want a struct", name, tn.Type().Underlying())
	}
	return tn, st
}

type wantField struct{ name, typ string }

func assertFields(t *testing.T, owner string, st *types.Struct, want []wantField) {
	t.Helper()
	var got []wantField
	for f := range st.Fields() {
		got = append(got, wantField{f.Name(), types.TypeString(f.Type(), func(p *types.Package) string { return p.Path() })})
	}
	if len(got) != len(want) {
		t.Fatalf("%s fields = %v, want exactly %v", owner, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s field %d = %v, want %v", owner, i, got[i], want[i])
		}
	}
}

func TestRemoved_FileSpecIsGone(t *testing.T) {
	if obj := loadEvo(t).Lookup("FileSpec"); obj != nil {
		t.Fatalf("evo.FileSpec still exists (%s); File{Content: Download{...}} replaces it", obj)
	}
}

func TestRemoved_FileIsATypeNotAFunction(t *testing.T) {
	obj := loadEvo(t).Lookup("File")
	if obj == nil {
		t.Fatal("evo.File does not exist")
	}
	if _, isFunc := obj.(*types.Func); isFunc {
		t.Fatal("evo.File is still the 1.1 function File(ctx, spec); it must be the File struct")
	}
}

func TestRemoved_FileHasNoContentsOrBasisFields(t *testing.T) {
	_, st := typeNamed(t, "File")
	for f := range st.Fields() {
		if f.Name() == "Contents" || f.Name() == "Basis" {
			t.Fatalf("evo.File still has the 1.1 field %s", f.Name())
		}
	}
}

func TestShape_FileIsAPlainStructWithContentAndMode(t *testing.T) {
	_, st := typeNamed(t, "File")
	assertFields(t, "evo.File", st, []wantField{
		{"Path", "string"},
		{"Content", evoPath + ".FileContent"},
		{"Mode", "io/fs.FileMode"},
	})
}

func TestShape_DownloadIsAPlainStructWithURLAndIntegrity(t *testing.T) {
	_, st := typeNamed(t, "Download")
	assertFields(t, "evo.Download", st, []wantField{{"URL", "string"}, {"Integrity", "string"}})
}

func TestShape_FileContentIsSealedAndDownloadSatisfiesIt(t *testing.T) {
	scope := loadEvo(t)
	obj := scope.Lookup("FileContent")
	if obj == nil {
		t.Fatal("evo.FileContent does not exist")
	}
	iface, ok := obj.Type().Underlying().(*types.Interface)
	if !ok {
		t.Fatalf("evo.FileContent underlies %s, want an interface", obj.Type().Underlying())
	}
	if iface.NumMethods() == 0 {
		t.Fatal("evo.FileContent has no method; an empty interface is not sealed")
	}
	for m := range iface.Methods() {
		if m.Exported() {
			t.Fatalf("evo.FileContent exports method %s; callers could implement it", m.Name())
		}
	}
	dl := scope.Lookup("Download")
	if dl == nil {
		t.Fatal("evo.Download does not exist")
	}
	if !types.Implements(dl.Type(), iface) {
		t.Fatal("evo.Download does not implement evo.FileContent")
	}
}

func TestShape_FileMethodsHaveValueReceivers(t *testing.T) {
	tn, _ := typeNamed(t, "File")
	set := types.NewMethodSet(tn.Type())
	want := map[string]string{
		"Write":  "func(ctx context.Context) error",
		"Verify": "func(ctx context.Context) error",
		"Remove": "func(ctx context.Context) error",
	}
	for name, sig := range want {
		sel := set.Lookup(evoPkg, name)
		if sel == nil {
			t.Fatalf("evo.File value method set lacks %s (receivers must be values)", name)
		}
		got := types.TypeString(sel.Type(), func(p *types.Package) string { return p.Name() })
		if got != sig {
			t.Fatalf("evo.File.%s = %s, want %s", name, got, sig)
		}
	}
}

func TestShape_BytesIsGenericOverStringAndByteSlice(t *testing.T) {
	obj := loadEvo(t).Lookup("Bytes")
	fn, ok := obj.(*types.Func)
	if !ok {
		t.Fatalf("evo.Bytes = %v, want a function", obj)
	}
	sig := fn.Type().(*types.Signature)
	if sig.TypeParams().Len() != 1 || sig.Params().Len() != 1 || sig.Results().Len() != 1 {
		t.Fatalf("evo.Bytes = %s, want func[T ~string | ~[]byte](v T) FileContent", sig)
	}
	if got := sig.Results().At(0).Type().String(); got != evoPath+".FileContent" {
		t.Fatalf("evo.Bytes returns %s, want FileContent", got)
	}
	constraint := sig.TypeParams().At(0).Constraint().Underlying().(*types.Interface)
	var terms []string
	for u := range constraint.EmbeddedTypes() {
		union, ok := u.(*types.Union)
		if !ok {
			continue
		}
		for term := range union.Terms() {
			terms = append(terms, map[bool]string{true: "~", false: ""}[term.Tilde()]+term.Type().String())
		}
	}
	if strings.Join(terms, "|") != "~string|~[]byte" {
		t.Fatalf("evo.Bytes constraint terms = %v, want ~string|~[]byte", terms)
	}
}

func TestShape_DownloadErrorsAreDeclared(t *testing.T) {
	for _, name := range []string{
		"ErrIntegrityMismatch", "ErrDownloadFailed", "ErrDownloadURLMissing",
		"ErrVerifyMismatch", "ErrPathMissing", "ErrContentMissing", "ErrNoTaskContext",
	} {
		obj := loadEvo(t).Lookup(name)
		v, ok := obj.(*types.Var)
		if !ok {
			t.Errorf("evo.%s = %v, want an exported error variable", name, obj)
			continue
		}
		if v.Type().String() != "error" {
			t.Errorf("evo.%s has type %s, want error", name, v.Type())
		}
	}
}
