package find_test

// Migration tests for Find. Nothing from 1.1 is superseded by Find: it is a
// new package-level discovery function, not a replacement for an old shape.
// So these tests pin the new surface to its exact kinds and assert that no
// competing spelling grows next to it (Find is discovery, not a File or Tree
// mode, and has no spec/option/finder types). They type-check the root evo
// package from source, so they detect absence without failing to compile.

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"sync"
	"testing"
)

const evoImportPath = "github.com/zachbornheimer/evident-output"

var (
	evoOnce sync.Once
	evoPkg  *types.Package
	evoErr  error
)

// loadEvo type-checks the non-test files of the root evo package.
func loadEvo(t *testing.T) *types.Package {
	t.Helper()
	evoOnce.Do(func() {
		root, err := filepath.Abs(filepath.Join("..", "..", ".."))
		if err != nil {
			evoErr = err
			return
		}
		bp, err := build.ImportDir(root, 0)
		if err != nil {
			evoErr = fmt.Errorf("list evo package files: %w", err)
			return
		}
		fset := token.NewFileSet()
		var files []*ast.File
		for _, name := range bp.GoFiles {
			f, err := parser.ParseFile(fset, filepath.Join(root, name), nil, 0)
			if err != nil {
				evoErr = fmt.Errorf("parse %s: %w", name, err)
				return
			}
			files = append(files, f)
		}
		imp := importer.ForCompiler(fset, "source", nil).(types.ImporterFrom)
		conf := types.Config{Importer: importerAt{imp, root}}
		evoPkg, evoErr = conf.Check(evoImportPath, fset, files, nil)
	})
	if evoErr != nil {
		t.Fatalf("type-check evo: %v", evoErr)
	}
	return evoPkg
}

// importerAt resolves imports relative to the module root so internal/
// packages are visible to the source importer.
type importerAt struct {
	types.ImporterFrom
	dir string
}

func (i importerAt) Import(path string) (*types.Package, error) {
	return i.ImportFrom(path, i.dir, 0)
}

// Names that would be a second way to do what evo.Find does.
var competingFindNames = []string{
	"FindSpec", "FindOption", "FindOptions", "FindResult", "Finder", "Matcher",
	"Glob", "Walk", "WalkDir", "Discover", "Search", "FindFiles", "FindAll",
}

func TestRemovedFindHasNoCompetingSpellings(t *testing.T) {
	scope := loadEvo(t).Scope()
	for _, name := range competingFindNames {
		if obj := scope.Lookup(name); obj != nil {
			t.Errorf("evo.%s exists (%s); discovery is the single function evo.Find", name, types.ObjectString(obj, nil))
		}
	}
}

func TestFindIsNotAFileOrTreeMode(t *testing.T) {
	pkg := loadEvo(t)
	for _, typeName := range []string{"File", "Tree"} {
		obj := pkg.Scope().Lookup(typeName)
		tn, ok := obj.(*types.TypeName)
		if !ok {
			t.Errorf("evo.%s is not a type (found %v); File/Tree must be plain structs", typeName, obj)
			continue
		}
		for _, banned := range []string{"Find", "Glob", "Match", "Pattern", "Names", "Walk", "Recursive"} {
			if o, _, _ := types.LookupFieldOrMethod(tn.Type(), true, pkg, banned); o != nil {
				t.Errorf("evo.%s has %s %q; discovery is not a %s mode", typeName, kindOf(o), banned, typeName)
			}
		}
	}
}

func kindOf(o types.Object) string {
	if _, ok := o.(*types.Func); ok {
		return "method"
	}
	return "field"
}

// The new identifiers exist with exactly the expected kinds.
func TestFindHasTheExactExpectedSurface(t *testing.T) {
	pkg := loadEvo(t)
	scope := pkg.Scope()

	fileObj, _ := scope.Lookup("File").(*types.TypeName)
	if fileObj == nil {
		t.Errorf("evo.File is not a type; Find returns []evo.File")
	}

	find, ok := scope.Lookup("Find").(*types.Func)
	if !ok {
		t.Errorf("evo.Find is %v, want a func", scope.Lookup("Find"))
	}
	if fileObj == nil || find == nil {
		return
	}
	fileType := fileObj.Type()
	ctxType := importedType(t, pkg, "context", "Context")
	params := types.NewTuple(
		types.NewVar(token.NoPos, nil, "ctx", ctxType),
		types.NewVar(token.NoPos, nil, "root", types.Typ[types.String]),
		types.NewVar(token.NoPos, nil, "names", types.NewSlice(types.Typ[types.String])),
	)
	results := types.NewTuple(
		types.NewVar(token.NoPos, nil, "", types.NewSlice(fileType)),
		types.NewVar(token.NoPos, nil, "", types.Universe.Lookup("error").Type()),
	)
	want := types.NewSignatureType(nil, nil, nil, params, results, true)
	if got := find.Type().(*types.Signature); !types.Identical(got, want) {
		t.Errorf("evo.Find has signature %s, want %s", got, want)
	}

	errObj, ok := scope.Lookup("ErrFindNamesMissing").(*types.Var)
	if !ok {
		t.Fatalf("evo.ErrFindNamesMissing is %v, want an error variable", scope.Lookup("ErrFindNamesMissing"))
	}
	if !types.Identical(errObj.Type(), types.Universe.Lookup("error").Type()) {
		t.Errorf("evo.ErrFindNamesMissing has type %s, want error", errObj.Type())
	}

	st, ok := fileType.Underlying().(*types.Struct)
	if !ok {
		t.Fatalf("evo.File is %s, want a plain struct", fileType.Underlying())
	}
	fields := map[string]bool{}
	for field := range st.Fields() {
		fields[field.Name()] = true
	}
	for _, name := range []string{"Path", "Content", "Mode"} {
		if !fields[name] {
			t.Errorf("evo.File has no field %s; Find returns Files addressed by Path with nil Content", name)
		}
	}
}

func importedType(t *testing.T, pkg *types.Package, path, name string) types.Type {
	t.Helper()
	for _, imp := range pkg.Imports() {
		if imp.Path() == path {
			if tn, ok := imp.Scope().Lookup(name).(*types.TypeName); ok {
				return tn.Type()
			}
		}
	}
	t.Fatalf("evo does not import %s.%s", path, name)
	return nil
}
