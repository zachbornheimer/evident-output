package architecture

import (
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestExportedSurfaceHasExampleCoverage fails, listing every gap, when an
// exported type or top-level function in the root evo package has no
// runnable Example function documenting it (go/doc's own naming
// convention: ExampleT for type T, ExampleF for function F — see
// https://pkg.go.dev/testing#hdr-Examples). Exported struct fields and
// package-level consts/vars are outside this gate: go/doc has no Example
// slot for them (there is no "ExampleValueName" convention), so a literal
// per-identifier reading is inexpressible for that half of the public
// surface and this gate covers the half that is.
func TestExportedSurfaceHasExampleCoverage(t *testing.T) {
	rootDir := moduleRoot(t)

	required := requiredExampleNames(t, rootDir)
	present := presentExampleNames(t, rootDir)

	var missing []string
	for name := range required {
		if !present[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		t.Fatalf("%d exported type(s)/function(s) have no Example%s function:\n  Example%s",
			len(missing), "<Name>", strings.Join(missing, "\n  Example"))
	}
}

// requiredExampleNames returns the "ExampleX" suffix (X) required for every
// exported type and top-level exported function declared in rootDir's
// non-test .go files.
func requiredExampleNames(t *testing.T, rootDir string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	matches, err := filepath.Glob(filepath.Join(rootDir, "*.go"))
	if err != nil {
		t.Fatalf("Glob(%s): %v", rootDir, err)
	}
	var files []*ast.File
	for _, name := range matches {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", name, err)
		}
		files = append(files, f)
	}
	docPkg, err := doc.NewFromFiles(fset, files, "github.com/zachbornheimer/evident-output", doc.AllDecls)
	if err != nil {
		t.Fatalf("NewFromFiles(%s): %v", rootDir, err)
	}

	required := make(map[string]bool)
	for _, typ := range docPkg.Types {
		if ast.IsExported(typ.Name) {
			required[typ.Name] = true
		}
		// go/doc groups a constructor-like func (e.g. func Command(...)
		// Action) under the type it returns instead of docPkg.Funcs — it is
		// still a plain top-level function, so its own name (not
		// "Type_Func") is the Example convention that applies to it.
		for _, fn := range typ.Funcs {
			if ast.IsExported(fn.Name) {
				required[fn.Name] = true
			}
		}
	}
	for _, fn := range docPkg.Funcs {
		if ast.IsExported(fn.Name) {
			required[fn.Name] = true
		}
	}
	return required
}

// presentExampleNames returns the "ExampleX" suffix (X) of every Example
// function go/doc recognizes in rootDir's _test.go files, keyed the same
// way requiredExampleNames is: a whole-type example (ExampleOutput) counts
// for that type, and go/doc also recognizes a disambiguating "_suffix" on
// an otherwise-package-level example, which this gate does not require and
// ignores.
func presentExampleNames(t *testing.T, rootDir string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	matches, err := filepath.Glob(filepath.Join(rootDir, "*_test.go"))
	if err != nil {
		t.Fatalf("Glob(%s): %v", rootDir, err)
	}
	var files []*ast.File
	for _, name := range matches {
		f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", name, err)
		}
		files = append(files, f)
	}

	present := make(map[string]bool)
	for _, ex := range doc.Examples(files...) {
		if ex.Name == "" {
			continue
		}
		// go/doc splits "TypeName_extraDisambiguator" into Name="TypeName"
		// with the rest available separately; for a bare top-level func or
		// type example (ExampleFoo), Name is already "Foo".
		present[ex.Name] = true
	}
	return present
}
