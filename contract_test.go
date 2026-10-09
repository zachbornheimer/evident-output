package evo_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestContractIsTheWholeSurface fails when package evo exports a name that
// CONTRACT.md does not list, or lacks a name that it does. CONTRACT.md is the
// source of truth; the package follows it, never the reverse.
func TestContractIsTheWholeSurface(t *testing.T) {
	want := contractSymbols(t, "CONTRACT.md")
	got := exportedSymbols(t, ".")

	missing := difference(want, got)
	extra := difference(got, want)

	if len(missing) > 0 {
		t.Errorf("CONTRACT.md lists %d symbols the package does not export:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
	if len(extra) > 0 {
		t.Errorf("package exports %d symbols CONTRACT.md does not list:\n  %s",
			len(extra), strings.Join(extra, "\n  "))
	}
}

// contractSymbol matches the first cell of a table row: | `Name` | or | `Type.Name` |.
var contractSymbol = regexp.MustCompile("^\\|\\s*`([A-Za-z_][A-Za-z0-9_]*(?:\\.[A-Za-z_][A-Za-z0-9_]*)?)`\\s*\\|")

func contractSymbols(t *testing.T, path string) map[string]bool {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	symbols := map[string]bool{}
	for line := range strings.SplitSeq(string(body), "\n") {
		m := contractSymbol.FindStringSubmatch(line)
		if m == nil || m[1] == "Symbol" {
			continue
		}
		symbols[m[1]] = true
	}
	if len(symbols) == 0 {
		t.Fatalf("%s: no symbol tables found", path)
	}
	return symbols
}

// exportedSymbols walks the non-test Go files of one package directory and
// returns every exported function, type, method, constant, variable and
// struct field. Interface method sets and alias targets are not expanded:
// an alias (`type X = other.Y`) counts as X with no fields, so a contract
// that lists X's fields stays red until the package owns the type.
func exportedSymbols(t *testing.T, dir string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	symbols := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			collectDecl(decl, symbols)
		}
	}
	return symbols
}

func collectDecl(decl ast.Decl, into map[string]bool) {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		collectFunc(d, into)
	case *ast.GenDecl:
		for _, spec := range d.Specs {
			collectSpec(spec, into)
		}
	}
}

func collectFunc(d *ast.FuncDecl, into map[string]bool) {
	if !d.Name.IsExported() {
		return
	}
	if d.Recv == nil {
		into[d.Name.Name] = true
		return
	}
	recv := receiverName(d.Recv.List[0].Type)
	if recv != "" && ast.IsExported(recv) {
		into[recv+"."+d.Name.Name] = true
	}
}

func collectSpec(spec ast.Spec, into map[string]bool) {
	switch s := spec.(type) {
	case *ast.ValueSpec:
		for _, name := range s.Names {
			if name.IsExported() {
				into[name.Name] = true
			}
		}
	case *ast.TypeSpec:
		if !s.Name.IsExported() {
			return
		}
		into[s.Name.Name] = true
		if st, ok := s.Type.(*ast.StructType); ok {
			collectFields(s.Name.Name, st, into)
		}
	}
}

func collectFields(typeName string, st *ast.StructType, into map[string]bool) {
	for _, field := range st.Fields.List {
		for _, name := range field.Names {
			if name.IsExported() {
				into[typeName+"."+name.Name] = true
			}
		}
	}
}

func receiverName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return receiverName(e.X)
	case *ast.Ident:
		return e.Name
	case *ast.IndexExpr: // generic receiver: Computed[T]
		return receiverName(e.X)
	case *ast.IndexListExpr:
		return receiverName(e.X)
	}
	return ""
}

func difference(a, b map[string]bool) []string {
	var out []string
	for name := range a {
		if !b[name] {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
