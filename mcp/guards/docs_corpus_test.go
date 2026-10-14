package guards_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/catalog"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/rules"
)

// agentsGuide is the operating contract AGENTS.md hands every agent that
// uses the MCP; it teaches the dialect, so the guards read it like any doc.
const agentsGuide = "AGENTS.md"

// docCorpus is every text the MCP serves or ships as guidance, keyed by a
// readable origin: AGENTS.md, docs, skills, the embedded sections, the catalog guides,
// the rule texts (BadCode and Replacement included: a mistake the rule
// shows is still written in 2.0 names), the retired table's replacement
// targets, and the review and adopt suggestions. A rule's BadOutput is left
// out: it is the program's rendering of the mistake.
func docCorpus(t *testing.T, root string) map[string]string {
	t.Helper()
	corpus := map[string]string{}
	agents, err := os.ReadFile(filepath.Join(root, agentsGuide))
	if err != nil {
		t.Fatalf("read %s: %v", agentsGuide, err)
	}
	corpus[agentsGuide] = string(agents)
	for _, dir := range []string{"mcp/docs", "mcp/skills", "mcp/internal/agent/sections/embedded"} {
		walkFiles(t, filepath.Join(root, dir), ".md", func(rel, body string) { corpus[filepath.ToSlash(rel)] = body })
	}
	for _, guide := range catalog.All() {
		corpus["catalog guide "+guide.ID] = guide.Body
	}
	for _, rule := range rules.All() {
		if rule.MaxDialect != "" {
			continue // teaches the dialect below its ceiling, on purpose
		}
		fields := map[string]string{
			"invariant": rule.Invariant, "why": rule.Why, "remediation": rule.Remediation,
			"replacement": rule.Replacement, "good code": rule.GoodCode, "good output": rule.GoodOutput,
			"exceptions": strings.Join(rule.Exceptions, "\n"),
		}
		if !flagsRemovedShape(rule) {
			fields["bad code"] = rule.BadCode
		}
		for field, text := range fields {
			corpus["rule "+rule.ID+" "+field] = text
		}
	}
	for _, symbol := range rules.RetiredSymbols() {
		corpus["retired replacement "+symbol.Contract] = symbol.Replacement
	}
	for _, dir := range []string{"mcp/internal/agent/review", "mcp/internal/agent/adopt"} {
		for site, text := range suggestionLiterals(t, filepath.Join(root, dir)) {
			corpus["suggestion "+site] = text
		}
	}
	for site, text := range detectorOutputs() {
		corpus["detector output "+site] = text
	}
	return corpus
}

// flagsRemovedShape reports whether the rule exists to flag an API that a
// release removed, which its own text says ("removed in 2.0"). Its BadCode
// is the removed shape on purpose; every other rule's BadCode is a mistake
// in 2.0 names.
func flagsRemovedShape(rule rules.Rule) bool {
	return removedNote.MatchString(rule.Invariant + " " + rule.Why + " " + rule.Remediation)
}

// walkFiles calls visit with the module-relative path and body of every file
// under dir that ends in suffix.
func walkFiles(t *testing.T, dir, suffix string, visit func(rel, body string)) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, suffix) {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(moduleRoot(t), path)
		visit(rel, string(body))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
}

// detectorFieldNames are the fields detector code assigns the fix text to.
var detectorFieldNames = map[string]bool{"Suggestion": true, "Remediation": true}

// detectorSuggestionCalls are the calls whose arguments carry the fix text
// (adopt's callSite.finding(rung, suggestion, certainty)).
var detectorSuggestionCalls = map[string]bool{"finding": true}

// suggestionLiterals are the string literals Go code in dir assigns to a
// Suggestion or Remediation field, or passes to a suggestion call, keyed by
// file:line. A value built by concatenation or fmt.Sprintf contributes every
// literal piece.
func suggestionLiterals(t *testing.T, dir string) map[string]string {
	t.Helper()
	literals := map[string]string{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
			strings.Contains(filepath.ToSlash(path), "/testdata/") {
			return err
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(moduleRoot(t), path)
		ast.Inspect(file, func(n ast.Node) bool {
			for _, value := range detectorValues(n) {
				if text := stringPieces(value); text != "" {
					literals[filepath.ToSlash(rel)+":"+strconv.Itoa(fset.Position(value.Pos()).Line)] = text
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("parse %s: %v", dir, err)
	}
	return literals
}

// detectorValues are the expressions n hands the fix text to: the value of a
// field named in detectorFieldNames (composite literal key or assignment), or
// the arguments of a call named in detectorSuggestionCalls.
func detectorValues(n ast.Node) []ast.Expr {
	switch n := n.(type) {
	case *ast.KeyValueExpr:
		if key, ok := n.Key.(*ast.Ident); ok && detectorFieldNames[key.Name] {
			return []ast.Expr{n.Value}
		}
	case *ast.AssignStmt:
		if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
			if sel, ok := n.Lhs[0].(*ast.SelectorExpr); ok && detectorFieldNames[sel.Sel.Name] {
				return n.Rhs
			}
		}
	case *ast.CallExpr:
		if sel, ok := n.Fun.(*ast.SelectorExpr); ok && detectorSuggestionCalls[sel.Sel.Name] {
			return n.Args
		}
	}
	return nil
}

// stringPieces joins every string literal inside expr, in source order.
func stringPieces(expr ast.Expr) string {
	var pieces []string
	ast.Inspect(expr, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			text, _ := strconv.Unquote(lit.Value)
			pieces = append(pieces, text)
		}
		return true
	})
	return strings.Join(pieces, "")
}

// retiredMutationSources are call sites of every removed mutation verb and
// every Record shape, in both removed argument orders, as the detectors see
// them in a reviewed file.
func retiredMutationSources() map[string]string {
	const header = "package p\nimport evo \"github.com/zachbornheimer/evident-output/v2\"\nfunc run(task *evo.TaskHandle, n int, path string) {\n  "
	const footer = "\n}\n"
	sources := map[string]string{}
	for _, verb := range []string{"Add", "Create", "Delete", "Push", "Remove", "Update", "Write"} {
		sources[verb+" quantity-first"] = header + `task.` + verb + `(n, "item")` + footer
		sources[verb+" object-first"] = header + `task.` + verb + `("item", func() error { return nil })` + footer
	}
	for _, spelling := range []string{"add", "create", "delete", "install", "push", "remove", "uninstall", "update", "write", "fetch-prune"} {
		sources["Record "+spelling] = header + `task.Record("` + spelling + `", n, "item")` + footer
		sources["RecordName "+spelling] = header + `task.RecordName("` + spelling + `", "item")` + footer
	}
	sources["RecordLabel"] = header + `task.RecordLabel("ready", n, "worker")` + footer
	return sources
}

// detectorOutputs are the Message and Suggestion texts the review detectors
// produce for retiredMutationSources, keyed by source and rule. These are
// computed from the call site's own identifiers, so no literal in the
// detector code holds them whole.
func detectorOutputs() map[string]string {
	outputs := map[string]string{}
	for name, src := range retiredMutationSources() {
		for _, finding := range review.GoSource("p.go", src).Findings {
			outputs[name+" "+finding.RuleID+" message"] = finding.Message
			outputs[name+" "+finding.RuleID+" suggestion"] = finding.Suggestion
		}
	}
	return outputs
}
