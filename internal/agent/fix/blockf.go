package fix

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// blockfVerb pairs a legacy printf-shaped TaskHandle verb with the 1.1
// statement verb the freeze kept: Block wins over Blockf, Fail wins over
// Failf (both are compatibility syntax, never taught per the vocabulary
// freeze).
type blockfVerb struct{ legacy, canonical string }

var blockfVerbs = []blockfVerb{
	{"Blockf", "Block"},
	{"Failf", "Fail"},
}

// BlockfAnalyzer fixes (*evo.TaskHandle).Blockf/Failf call sites.
var BlockfAnalyzer = &analysis.Analyzer{
	Name:     "evoblockf",
	Doc:      "flags and fixes evo TaskHandle.Blockf/Failf, legacy syntax for Block/Fail",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runBlockf,
}

func runBlockf(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.WithStack([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		call := n.(*ast.CallExpr)
		sel := callSelector(call)
		if sel == nil {
			return true
		}
		verb := matchBlockfVerb(sel.Sel.Name)
		if verb == "" {
			return true
		}
		if recv, ok := recvNamedType(pass.TypesInfo, sel.X); !ok || recv != "TaskHandle" {
			return true
		}
		pass.Report(blockfFinding(pass, call, sel, verb, stack))
		return true
	})
	return nil, nil
}

func matchBlockfVerb(name string) string {
	for _, v := range blockfVerbs {
		if v.legacy == name {
			return v.canonical
		}
	}
	return ""
}

// blockfFinding classifies the call's syntactic position (a bare
// statement, a Define-callback return, a chained .Next(...), or anything
// else) and builds the matching diagnostic. Every shape outside the three
// mechanical ones gets a diagnostic with no fix rather than a guessed
// rewrite.
func blockfFinding(pass *analysis.Pass, call *ast.CallExpr, sel *ast.SelectorExpr, canonical string, stack []ast.Node) analysis.Diagnostic {
	msg := "evo.TaskHandle." + sel.Sel.Name + " is legacy syntax: " + canonical + " wins, never taught"
	category := "API-080"
	if sel.Sel.Name == "Failf" {
		category = "API-081"
	}

	parent, grandparent := parentNodes(stack)

	// t.Blockf(f, args...).Next(x) — fold Next into a Block ProblemOption.
	if outer, ok := parent.(*ast.CallExpr); ok {
		if outerSel, ok := outer.Fun.(*ast.SelectorExpr); ok && outerSel.X == call && outerSel.Sel.Name == "Next" && len(outer.Args) == 1 {
			return diag(category, outer, msg, blockfNextFix(pass, call, sel, outer, outerSel, canonical))
		}
	}

	// return t.Blockf(f, args...) — only mechanical inside a Define
	// callback, func(context.Context) error; anywhere else the enclosing
	// function's error contract is unknown, so "return err" is never
	// guessed.
	if ret, ok := parent.(*ast.ReturnStmt); ok && len(ret.Results) == 1 && ret.Results[0] == call {
		if isDefineCallback(exprEnclosingFunc(stack)) {
			return diag(category, ret, msg, blockfReturnFix(pass, call, sel, ret, canonical))
		}
		return diag(category, ret, msg+" — not rewritten: a return outside a Define callback has no known error contract to satisfy")
	}

	// t.Blockf(f, args...) as a bare statement.
	if _, ok := parent.(*ast.ExprStmt); ok {
		_ = grandparent
		return diag(category, call, msg, blockfStatementFix(pass, call, sel, canonical))
	}

	return diag(category, call, msg+" — not rewritten: call is neither a statement nor a Define-callback return")
}

func parentNodes(stack []ast.Node) (parent, grandparent ast.Node) {
	if n := len(stack); n >= 2 {
		parent = stack[n-2]
	}
	if n := len(stack); n >= 3 {
		grandparent = stack[n-3]
	}
	return parent, grandparent
}

// isDefineCallback reports whether fn has the Define callback shape,
// func(context.Context) error.
func isDefineCallback(fn *ast.FuncLit) bool {
	if fn == nil {
		return false
	}
	sig := fn.Type
	if sig.Params == nil || len(sig.Params.List) != 1 {
		return false
	}
	if sig.Results == nil || len(sig.Results.List) != 1 {
		return false
	}
	resultIdent, ok := sig.Results.List[0].Type.(*ast.Ident)
	return ok && resultIdent.Name == "error"
}

// sprintfExpr renders fmt.Sprintf(f, args...) from a Blockf/Failf call's
// own arguments, verbatim from source.
func sprintfExpr(pass *analysis.Pass, call *ast.CallExpr) string {
	args := argTexts(pass, call.Args)
	var joined strings.Builder
	for i, a := range args {
		if i > 0 {
			joined.WriteString(", ")
		}
		joined.WriteString(a)
	}
	return "fmt.Sprintf(" + joined.String() + ")"
}

// blockfStatementFix rewrites `t.Blockf(f, args...)` used as a bare
// statement to `t.Block(fmt.Sprintf(f, args...))`.
func blockfStatementFix(pass *analysis.Pass, call *ast.CallExpr, sel *ast.SelectorExpr, canonical string) analysis.SuggestedFix {
	recv := text(pass, sel.X)
	newText := recv + "." + canonical + "(" + sprintfExpr(pass, call) + ")"
	return analysis.SuggestedFix{
		Message: "replace " + sel.Sel.Name + " with " + canonical + "(fmt.Sprintf(...))",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: call.End(), NewText: []byte(newText)},
			addFmtImport(pass, call.Pos()),
		},
	}
}

// blockfReturnFix rewrites `return t.Blockf(f, args...)` inside a Define
// callback to `t.Block(fmt.Sprintf(f, args...)); return <tail>` — where
// <tail> is `fmt.Errorf(f, args...)` when the format literal carries %w
// (the refusal also needs to wrap an underlying error) and `nil` otherwise
// (the Task's own Blocked outcome, set by Block, is the signal; the
// Define callback returning nil does not mean it succeeded once the Task
// already resolved Blocked).
func blockfReturnFix(pass *analysis.Pass, call *ast.CallExpr, sel *ast.SelectorExpr, ret *ast.ReturnStmt, canonical string) analysis.SuggestedFix {
	recv := text(pass, sel.X)
	stmt := recv + "." + canonical + "(" + sprintfExpr(pass, call) + ")"
	tail := "nil"
	if len(call.Args) > 0 && hasVerbW(pass, call.Args[0]) {
		tail = "fmt.Errorf(" + joinArgTexts(pass, call.Args) + ")"
	}
	newText := stmt + "\n\treturn " + tail
	return analysis.SuggestedFix{
		Message: "replace return " + sel.Sel.Name + "(...) with " + canonical + "(...); return " + tail,
		TextEdits: []analysis.TextEdit{
			{Pos: ret.Pos(), End: ret.End(), NewText: []byte(newText)},
			addFmtImport(pass, ret.Pos()),
		},
	}
}

// blockfNextFix rewrites `t.Blockf(f, args...).Next(x)` to
// `t.Block(fmt.Sprintf(f, args...), evo.Next(x))`, folding the chained
// Next call into a Block ProblemOption instead of leaving it dangling on
// a *Failure that Block never returns.
func blockfNextFix(pass *analysis.Pass, call *ast.CallExpr, sel *ast.SelectorExpr, outer *ast.CallExpr, outerSel *ast.SelectorExpr, canonical string) analysis.SuggestedFix {
	recv := text(pass, sel.X)
	alias := evoAlias(pass, sel)
	nextArg := text(pass, outer.Args[0])
	newText := recv + "." + canonical + "(" + sprintfExpr(pass, call) + ", " + alias + ".Next(" + nextArg + "))"
	return analysis.SuggestedFix{
		Message: "fold .Next(...) into " + canonical + "'s ProblemOption",
		TextEdits: []analysis.TextEdit{
			{Pos: outer.Pos(), End: outer.End(), NewText: []byte(newText)},
			addFmtImport(pass, outer.Pos()),
		},
	}
}

func joinArgTexts(pass *analysis.Pass, args []ast.Expr) string {
	var joined strings.Builder
	for i, a := range args {
		if i > 0 {
			joined.WriteString(", ")
		}
		joined.WriteString(text(pass, a))
	}
	return joined.String()
}

// addFmtImport inserts `import "fmt"` right after the package clause when
// the file does not already import it. A repeated insertion (one Block
// fix per diagnostic in the same file) is harmless: applying edits is
// idempotent per file because gofmt/goimports normalizes the result, and
// the CLI runs goimports-equivalent dedup via `-apply`'s formatting pass.
func addFmtImport(pass *analysis.Pass, at token.Pos) analysis.TextEdit {
	f := enclosingFile(pass, at)
	if f == nil {
		return analysis.TextEdit{}
	}
	for _, imp := range f.Imports {
		if importPath(imp) == "fmt" {
			return analysis.TextEdit{Pos: f.Package, End: f.Package}
		}
	}
	return analysis.TextEdit{
		Pos:     f.Name.End(),
		End:     f.Name.End(),
		NewText: []byte("\n\nimport \"fmt\""),
	}
}

func enclosingFile(pass *analysis.Pass, at token.Pos) *ast.File {
	for _, f := range pass.Files {
		if f.Pos() <= at && at <= f.End() {
			return f
		}
	}
	return nil
}
