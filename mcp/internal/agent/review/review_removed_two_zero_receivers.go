package review

import (
	"go/ast"
	"go/token"
	"slices"
	"strings"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/rules"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/modpin"
)

// removedReceiver is which 1.x evo type a method call is made on, as far as
// the file proves it: the review package loads no imports, so a receiver is
// reported only when its binding is visible here.
type removedReceiver int

const (
	unknownReceiver removedReceiver = iota
	taskReceiver
	outputReceiver
	failureReceiver
)

// label is the 2.0 name for what the receiver was; a removed method is
// reported on it, never on the 1.x handle type.
func (r removedReceiver) label() string {
	switch r {
	case taskReceiver:
		return "Task"
	case outputReceiver:
		return "Output"
	case failureReceiver:
		return "Failure"
	}
	return ""
}

// legacyType is the retired table's spelling of the receiver's 1.x type.
func (r removedReceiver) legacyType() string {
	if r == taskReceiver {
		return "TaskHandle"
	}
	return r.label()
}

func (d *removedTwoZeroDetector) receiverOf(x ast.Expr) removedReceiver {
	switch {
	case d.tasks.IsTask(x):
		return taskReceiver
	case d.outputs.has(x), isOutputConstructor(x, d.pkg):
		return outputReceiver
	case d.failures.has(x):
		return failureReceiver
	}
	return unknownReceiver
}

// removedIn is the release that removed method on recv, from the retired
// table; 2.0 when the table has no row for it.
func removedIn(recv removedReceiver, method string) rules.RetiredRelease {
	for _, key := range []string{recv.legacyType() + "." + method + "(", method} {
		for _, s := range rules.RetiredSymbols() {
			if s.Contract == key {
				return s.RemovedIn
			}
		}
	}
	return rules.RetiredRelease2_0
}

// removedPhrase is "<Recv>.<method> was removed in <release>".
func removedPhrase(recv removedReceiver, method string) string {
	return recv.label() + "." + method + " was removed in " + string(removedIn(recv, method))
}

// failureBindings is which names in one file are typed evo.Failure.
type failureBindings struct {
	names map[string]bool
}

func newFailureBindings(file *ast.File, evoPkg string) failureBindings {
	b := failureBindings{names: map[string]bool{}}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Field:
			if isFailureType(n.Type, evoPkg) {
				for _, name := range n.Names {
					b.names[name.Name] = true
				}
			}
		case *ast.ValueSpec:
			if n.Type != nil && isFailureType(n.Type, evoPkg) {
				for _, name := range n.Names {
					b.names[name.Name] = true
				}
			}
		}
		return true
	})
	return b
}

func (b failureBindings) has(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	return ok && b.names[id.Name]
}

func isFailureType(t ast.Expr, evoPkg string) bool {
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	switch x := t.(type) {
	case *ast.SelectorExpr:
		return x.Sel.Name == "Failure" && isEvoIdent(x.X, evoPkg)
	case *ast.Ident:
		return x.Name == "Failure"
	}
	return false
}

// defineBody is one `recv.Define(func ...)` callback literal's span.
type defineBody struct {
	recv string
	span srcSpan
}

// collectDefineBodies finds every `recv.Define(func ...)` callback literal.
func collectDefineBodies(f *ast.File, fset *token.FileSet) []defineBody {
	var bodies []defineBody
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Define" || len(call.Args) != 1 {
			return true
		}
		if lit, ok := call.Args[0].(*ast.FuncLit); ok {
			file := fset.File(lit.Pos())
			span := srcSpan{start: file.Offset(lit.Pos()), end: file.Offset(lit.End())}
			bodies = append(bodies, defineBody{recv: exprDottedName(sel.X), span: span})
		}
		return true
	})
	return bodies
}

// insideOwnDefine reports whether offset sits in recv's own Define body.
func (d *removedTwoZeroDetector) insideOwnDefine(recv string, offset int) bool {
	return slices.ContainsFunc(d.defineBodies, func(b defineBody) bool {
		return b.recv == recv && b.span.contains(offset)
	})
}

// evoImportName is the local name of the evo import, or "" when the file
// does not import it.
func evoImportName(f *ast.File) string {
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if !modpin.IsModulePath(path) {
			continue
		}
		if imp.Name != nil && imp.Name.Name != "." && imp.Name.Name != "_" {
			return imp.Name.Name
		}
		return "evo"
	}
	return ""
}

func isEvoIdent(e ast.Expr, pkg string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == pkg
}

func identName(e ast.Expr) string {
	id, ok := e.(*ast.Ident)
	if !ok {
		return ""
	}
	return id.Name
}
