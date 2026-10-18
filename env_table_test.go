package evo_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestEnvTableListsEveryVariableRead holds CONTRACT.md §6's "these are the
// only variables Evo reads" to the source: every name read through the env
// facade (process.Env, the root's lookupEnv, or os.Getenv/os.LookupEnv) is a
// row of the §6 table, and every row is read somewhere.
func TestEnvTableListsEveryVariableRead(t *testing.T) {
	read := environmentReads(t)
	if len(read) == 0 {
		t.Fatal("found no environment reads: the scan is looking in the wrong place")
	}
	listed := contractEnvironmentTable(t, "CONTRACT.md")

	for _, name := range sortedKeys(read) {
		if !listed[name] {
			t.Errorf("%s is read at %s but is not a row of the CONTRACT.md §6 environment table", name, read[name])
		}
	}
	for _, name := range sortedKeys(listed) {
		if _, ok := read[name]; !ok {
			t.Errorf("CONTRACT.md §6 lists %s but no non-test code in the root or internal packages reads it", name)
		}
	}
}

// contractEnvironmentTable is the Variable column of the §6 table.
func contractEnvironmentTable(t *testing.T, path string) map[string]bool {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	_, section, ok := strings.Cut(string(body), "\n## 6. ")
	if !ok {
		t.Fatalf("%s has no section 6", path)
	}
	section, _, _ = strings.Cut(section, "\n## 7. ")
	names := map[string]bool{}
	inTable := false
	for line := range strings.SplitSeq(section, "\n") {
		if !strings.HasPrefix(line, "|") {
			inTable = false
			continue
		}
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		name := strings.Trim(strings.TrimSpace(cells[0]), "`")
		switch {
		case name == "Variable":
			inTable = true
		case inTable && strings.Trim(name, "-: ") != "":
			names[name] = true
		}
	}
	return names
}

// environmentReads maps each variable name read in non-test code of the root
// and internal packages to the first position that reads it. A read whose
// name is neither a literal nor a package-level constant fails the test: the
// table cannot be checked against a name the source does not spell out.
func environmentReads(t *testing.T) map[string]string {
	t.Helper()
	reads := map[string]string{}
	for _, dir := range packageDirs(t) {
		fset := token.NewFileSet()
		files := parsePackage(t, fset, dir)
		consts := stringConstants(files)
		for _, file := range files {
			if strings.HasSuffix(filepath.ToSlash(fset.Position(file.Pos()).Filename), "internal/process/env.go") {
				continue // defines the facade; its argument is the caller's
			}
			ast.Inspect(file, func(n ast.Node) bool {
				if fn, ok := n.(*ast.FuncDecl); ok && fn.Name.Name == "lookupEnv" {
					return false // the root's wrapper forwards its caller's name
				}
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) != 1 || !isEnvironmentLookup(call.Fun) {
					return true
				}
				at := fset.Position(call.Pos()).String()
				name, ok := stringValue(call.Args[0], consts)
				if !ok {
					t.Errorf("%s: environment read with a name that is not a literal or constant", at)
					return true
				}
				if _, seen := reads[name]; !seen {
					reads[name] = at
				}
				return true
			})
		}
	}
	return reads
}

// isEnvironmentLookup reports whether fn is the env facade or an os lookup.
func isEnvironmentLookup(fn ast.Expr) bool {
	switch f := fn.(type) {
	case *ast.Ident:
		return f.Name == "lookupEnv"
	case *ast.SelectorExpr:
		pkg, ok := f.X.(*ast.Ident)
		if !ok {
			return false
		}
		return (pkg.Name == "process" && f.Sel.Name == "Env") ||
			(pkg.Name == "os" && (f.Sel.Name == "Getenv" || f.Sel.Name == "LookupEnv"))
	}
	return false
}

// stringValue is the string expr spells: a literal, or a constant of the package.
func stringValue(expr ast.Expr, consts map[string]string) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		return s, err == nil
	case *ast.Ident:
		s, ok := consts[e.Name]
		return s, ok
	}
	return "", false
}

// stringConstants is every package-level `name = "literal"` constant.
func stringConstants(files []*ast.File) map[string]string {
	consts := map[string]string{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, id := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					if s, ok := stringValue(vs.Values[i], nil); ok {
						consts[id.Name] = s
					}
				}
			}
		}
	}
	return consts
}

// packageDirs is the root directory and every directory under internal/.
func packageDirs(t *testing.T) []string {
	t.Helper()
	dirs := []string{"."}
	err := filepath.WalkDir("internal", func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, path)
		}
		return err
	})
	if err != nil {
		t.Fatalf("walk internal: %v", err)
	}
	return dirs
}

// parsePackage parses the non-test Go files of one directory.
func parsePackage(t *testing.T, fset *token.FileSet, dir string) []*ast.File {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var files []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", filepath.Join(dir, name), err)
		}
		files = append(files, file)
	}
	return files
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
