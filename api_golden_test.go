package evo_test

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
	"testing"
)

// api_golden_test.go is §46's public API golden: it lists every exported
// identifier of the root evo package — types, functions, constants,
// variables, and each exported type's exported methods and struct fields —
// against the committed golden file testdata/api_golden.txt. Any addition,
// removal, or rename to the public surface changes this golden and must be
// a deliberate, reviewed diff; it also fails outright if any of the
// retired 1.0.0 names reappear (retiredAPINames below), regardless of what
// the committed golden says.
//
// Unlike dialect_surface_test.go (funcs/methods for a hand-picked set of
// receivers, checked against a literal Go map), this golden is exhaustive
// and mechanical: it walks every exported declaration go/doc finds, so a
// newly added exported type or top-level func is caught even before anyone
// remembers to add it to a hand-picked list.

const apiGoldenPath = "testdata/api_golden.txt"

// retiredAPINames are identifiers 1.0.0 deliberately removed (MainWith,
// Each) or never had (speculative names from earlier drafts/other
// libraries) — a reappearance fails this test even if someone regenerated
// testdata/api_golden.txt to match, so retiring a name stays retired.
var retiredAPINames = []string{
	"Task.Run", "Task.Go", "Task.Each",
	"DisplayGroup",
	"Group.Done",
	"Sequence.Fail",
	"TaskConfig",
	"MainWith",
}

// buildAPISurface parses every non-test .go file in dir (the root package)
// and renders one sorted, deterministic line per exported identifier: a
// type declaration, its exported struct fields, its exported methods, then
// top-level exported funcs, consts, and vars — go/doc's own grouping, so a
// rename or a new exported symbol always lands in a stable, reviewable
// place in the diff.
func buildAPISurface(t *testing.T, dir string) []string {
	t.Helper()
	fset := token.NewFileSet()
	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("Glob(%s): %v", dir, err)
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
		t.Fatalf("NewFromFiles(%s): %v", dir, err)
	}

	var lines []string
	for _, typ := range docPkg.Types {
		if !ast.IsExported(typ.Name) {
			continue
		}
		lines = append(lines, "type "+typ.Name)
		for _, field := range exportedStructFields(typ) {
			lines = append(lines, "type "+typ.Name+"."+field)
		}
		for _, m := range typ.Methods {
			if ast.IsExported(m.Name) {
				lines = append(lines, "func ("+typ.Name+") "+m.Name+renderParams(fset, m.Decl.Type))
			}
		}
		// go/doc groups a top-level func under its return type's Funcs
		// (constructor-style grouping) instead of docPkg.Funcs whenever its
		// first result is that type — e.g. func ParseFormat(string) (Format,
		// error) lands here, not below. Missing this loop let such a func
		// join the public surface invisibly to this golden.
		for _, fn := range typ.Funcs {
			if ast.IsExported(fn.Name) {
				lines = append(lines, "func "+fn.Name+renderParams(fset, fn.Decl.Type))
			}
		}
	}
	for _, fn := range docPkg.Funcs {
		if ast.IsExported(fn.Name) {
			lines = append(lines, "func "+fn.Name+renderParams(fset, fn.Decl.Type))
		}
	}
	for _, group := range [][]*doc.Value{docPkg.Consts, docPkg.Vars} {
		for _, v := range group {
			for _, name := range v.Names {
				if ast.IsExported(name) {
					lines = append(lines, "value "+name)
				}
			}
		}
	}
	sort.Strings(lines)
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

// exportedStructFields returns "Name Type" for each exported field of typ,
// when typ's declaration is a struct — empty for an interface, a func type,
// or an alias.
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

func TestAPIGolden_PublicSurfaceMatchesCommittedGolden(t *testing.T) {
	got := buildAPISurface(t, ".")

	if os.Getenv("UPDATE_API_GOLDEN") == "1" {
		if err := os.WriteFile(apiGoldenPath, []byte(strings.Join(got, "\n")+"\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", apiGoldenPath, err)
		}
	}

	for _, retired := range retiredAPINames {
		for _, line := range got {
			if strings.Contains(line, retired) {
				t.Fatalf("retired API name %q reappeared in the public surface: %q", retired, line)
			}
		}
	}

	wantBytes, err := os.ReadFile(apiGoldenPath)
	if err != nil {
		t.Fatalf("read %s: %v", apiGoldenPath, err)
	}
	want := strings.Split(strings.TrimRight(string(wantBytes), "\n"), "\n")

	if diff := diffLines(want, got); diff != "" {
		t.Fatalf("public API surface != %s (regenerate deliberately if this is an intended change):\n%s", apiGoldenPath, diff)
	}
}

// diffLines reports a human-readable added/removed summary, or "" when want
// and got are identical.
func diffLines(want, got []string) string {
	if strings.Join(want, "\n") == strings.Join(got, "\n") {
		return ""
	}
	wantSet := make(map[string]bool, len(want))
	for _, w := range want {
		wantSet[w] = true
	}
	gotSet := make(map[string]bool, len(got))
	for _, g := range got {
		gotSet[g] = true
	}
	var added, removed []string
	for _, g := range got {
		if !wantSet[g] {
			added = append(added, g)
		}
	}
	for _, w := range want {
		if !gotSet[w] {
			removed = append(removed, w)
		}
	}
	var b strings.Builder
	if len(added) > 0 {
		fmt.Fprintf(&b, "ADDED (%d):\n  %s\n", len(added), strings.Join(added, "\n  "))
	}
	if len(removed) > 0 {
		fmt.Fprintf(&b, "REMOVED (%d):\n  %s\n", len(removed), strings.Join(removed, "\n  "))
	}
	return b.String()
}
