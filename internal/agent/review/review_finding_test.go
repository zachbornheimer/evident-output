package review

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/rules"
)

// TestEveryEmittedRuleIDIsInTheCatalog is what lets finalize own severity:
// every RuleID a detector can emit (a literal, or a package constant) must
// name a catalog rule, or its findings would ship with no severity.
func TestEveryEmittedRuleIDIsInTheCatalog(t *testing.T) {
	ids := emittedRuleIDs(t)
	if len(ids) < 50 {
		t.Fatalf("found only %d emitted rule IDs; the scan is broken", len(ids))
	}
	for id, where := range ids {
		if _, ok := rules.SeverityOf(id); !ok {
			t.Errorf("%s: RuleID %q has no rules catalog entry", where, id)
		}
	}
}

// TestFinalize_TakesSeverityFromTheCatalog pins the one owner of severity.
func TestFinalize_TakesSeverityFromTheCatalog(t *testing.T) {
	got := finalize([]Finding{{RuleID: "API-055"}, {RuleID: "API-000"}, {RuleID: "EVO-FILE-001", Severity: "warning"}})
	want := []string{"warning", "error", "warning"}
	for i, f := range got {
		if f.Severity != want[i] {
			t.Errorf("%s severity = %q, want %q", f.RuleID, f.Severity, want[i])
		}
	}
}

// emittedRuleIDs maps each rule ID the package's non-test sources can put
// in a Finding's RuleID to one source position that does.
func emittedRuleIDs(t *testing.T) map[string]string {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	consts := map[string]string{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
		ast.Inspect(f, func(n ast.Node) bool {
			vs, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for i, v := range vs.Values {
				if lit, ok := v.(*ast.BasicLit); ok && lit.Kind == token.STRING && i < len(vs.Names) {
					consts[vs.Names[i].Name], _ = strconv.Unquote(lit.Value)
				}
			}
			return true
		})
	}
	ids := map[string]string{}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			kv, ok := n.(*ast.KeyValueExpr)
			if !ok {
				return true
			}
			if key, ok := kv.Key.(*ast.Ident); !ok || key.Name != "RuleID" {
				return true
			}
			var id string
			switch v := kv.Value.(type) {
			case *ast.BasicLit:
				id, _ = strconv.Unquote(v.Value)
			case *ast.Ident:
				id = consts[v.Name]
			}
			if id != "" {
				ids[id] = fset.Position(kv.Pos()).String()
			}
			return true
		})
	}
	return ids
}
