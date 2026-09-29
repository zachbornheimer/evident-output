package apisurface

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/modpin"
)

// Walk parses every non-test .go file in dir (the root package) and
// renders one sorted, deterministic line per exported identifier: a type
// declaration, its exported struct fields, its exported methods, then
// top-level exported funcs, consts, and vars — go/doc's own grouping, so
// a rename or a new exported symbol always lands in a stable, reviewable
// place in the diff.
func Walk(dir string) ([]string, error) {
	fset := token.NewFileSet()
	docPkg, err := parsePackageDoc(fset, dir)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, typ := range docPkg.Types {
		lines = append(lines, typeLines(fset, typ)...)
	}
	lines = append(lines, funcLines(fset, docPkg.Funcs)...)
	for _, group := range [][]*doc.Value{docPkg.Consts, docPkg.Vars} {
		lines = append(lines, valueLines(group)...)
	}
	sort.Strings(lines)
	return lines, nil
}

// parsePackageDoc parses every non-test .go file in dir into go/doc's
// view of the package, keeping unexported declarations so grouping
// matches the source.
func parsePackageDoc(fset *token.FileSet, dir string) (*doc.Package, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("apisurface: stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("apisurface: %s is not a directory", dir)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, fmt.Errorf("apisurface: glob %s: %w", dir, err)
	}
	var files []*ast.File
	for _, name := range matches {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("apisurface: parse %s: %w", name, err)
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("apisurface: no Go source in %s", dir)
	}
	docPkg, err := doc.NewFromFiles(fset, files, modpin.ModulePath, doc.AllDecls)
	if err != nil {
		return nil, fmt.Errorf("apisurface: doc %s: %w", dir, err)
	}
	return docPkg, nil
}

// typeLines is one exported type's surface: the type, its exported struct
// fields and methods, and the funcs go/doc groups under it. go/doc files a
// top-level func under its return type's Funcs (constructor-style
// grouping) whenever its first result is that type — e.g. func
// ParseFormat(string) (Format, error) lands here — so missing them would
// let such a func join the public surface invisibly to this golden.
func typeLines(fset *token.FileSet, typ *doc.Type) []string {
	if !ast.IsExported(typ.Name) {
		return nil
	}
	lines := []string{"type " + typ.Name}
	for _, field := range exportedStructFields(typ) {
		lines = append(lines, "type "+typ.Name+"."+field)
	}
	for _, m := range typ.Methods {
		if ast.IsExported(m.Name) {
			lines = append(lines, "func ("+typ.Name+") "+m.Name+renderParams(fset, m.Decl.Type))
		}
	}
	return append(lines, funcLines(fset, typ.Funcs)...)
}

// funcLines is each exported func with its signature.
func funcLines(fset *token.FileSet, funcs []*doc.Func) []string {
	var lines []string
	for _, fn := range funcs {
		if ast.IsExported(fn.Name) {
			lines = append(lines, "func "+fn.Name+renderParams(fset, fn.Decl.Type))
		}
	}
	return lines
}

// valueLines is each exported const or var name.
func valueLines(values []*doc.Value) []string {
	var lines []string
	for _, v := range values {
		for _, name := range v.Names {
			if ast.IsExported(name) {
				lines = append(lines, "value "+name)
			}
		}
	}
	return lines
}

// renderParams renders a func/method's parameter and result list exactly as
// written (names and types included), so a signature change shows up as a
// line change instead of silently matching a same-named, differently-typed
// declaration. go/printer prints an individual field's type node cleanly
// but not a bare *ast.FieldList (it renders empty without the enclosing
// parens a func literal normally supplies), so this renders field by field.
func renderParams(fset *token.FileSet, ft *ast.FuncType) string {
	sig := "(" + renderFieldList(fset, ft.Params) + ")"
	if results := renderFieldList(fset, ft.Results); results != "" {
		sig += " " + results
	}
	return sig
}

// renderFieldList renders each field as "name1, name2 Type", comma-joined —
// nil (no parameter/result list) renders as "".
func renderFieldList(fset *token.FileSet, fl *ast.FieldList) string {
	if fl == nil {
		return ""
	}
	parts := make([]string, 0, len(fl.List))
	for _, f := range fl.List {
		var typeBuf bytes.Buffer
		_ = printer.Fprint(&typeBuf, fset, f.Type)
		names := make([]string, len(f.Names))
		for i, n := range f.Names {
			names[i] = n.Name
		}
		parts = append(parts, strings.Join(names, ", ")+" "+typeBuf.String())
	}
	return strings.Join(parts, ", ")
}

// exportedStructFields returns the exported field names of typ when typ's
// declaration is a struct — empty for an interface, a func type, or an alias.
func exportedStructFields(typ *doc.Type) []string {
	var fields []string
	for _, spec := range typ.Decl.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok || st.Fields == nil {
			continue
		}
		for _, f := range st.Fields.List {
			for _, name := range f.Names {
				if ast.IsExported(name.Name) {
					fields = append(fields, name.Name)
				}
			}
		}
	}
	return fields
}
