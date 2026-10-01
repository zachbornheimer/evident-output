package review

import (
	"go/ast"
	"strings"
)

// remedyStatement is a call that stands alone as a statement, and the block
// that directly holds it (nil inside a case clause).
type remedyStatement struct {
	block *ast.BlockStmt
	index int
}

// standaloneStatement finds call as an expression statement. A call that is
// part of a larger expression (a fluent chain, an assignment) has no
// statement-level rewrite, so it returns false.
func (d *recSurfaceDetector) standaloneStatement(call *ast.CallExpr) (remedyStatement, bool) {
	var found remedyStatement
	ok := false
	ast.Inspect(d.file, func(n ast.Node) bool {
		if ok {
			return false
		}
		switch n := n.(type) {
		case *ast.BlockStmt:
			for i, st := range n.List {
				if es, isExpr := st.(*ast.ExprStmt); isExpr && es.X == call {
					found, ok = remedyStatement{block: n, index: i}, true
					return false
				}
			}
		case *ast.ExprStmt:
			if n.X == call {
				found, ok = remedyStatement{}, true
			}
		}
		return !ok
	})
	return found, ok
}

// siblingDiagnostic is the Fail/Block/Problem statement on exactly the same
// receiver in the same block as the Next statement, with no spread arguments:
// the only place a remedy can fold without changing which path it belongs to.
// A preceding statement wins over a following one.
func (d *recSurfaceDetector) siblingDiagnostic(s remedyStatement, recvSrc string) *ast.CallExpr {
	if s.block == nil {
		return nil
	}
	list := s.block.List
	for i := s.index - 1; i >= 0; i-- {
		if c := d.plainDiagnostic(list[i], recvSrc); c != nil {
			return c
		}
	}
	for i := s.index + 1; i < len(list); i++ {
		if c := d.plainDiagnostic(list[i], recvSrc); c != nil {
			return c
		}
	}
	return nil
}

// plainDiagnostic is st's call when st is `recv.Fail|Block|Problem(...)` with
// no spread argument.
func (d *recSurfaceDetector) plainDiagnostic(st ast.Stmt, recvSrc string) *ast.CallExpr {
	es, ok := st.(*ast.ExprStmt)
	if !ok {
		return nil
	}
	call, ok := es.X.(*ast.CallExpr)
	if !ok || call.Ellipsis.IsValid() {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !isDiagnosticVerb(sel.Sel.Name) || d.nodeSrc(sel.X) != recvSrc {
		return nil
	}
	return call
}

// foldSuggestion is one replace edit that spans the diagnostic statement
// through the Next statement: the diagnostic gains the options, the Next
// statement disappears, and whatever sat between them is kept in order.
func (d *recSurfaceDetector) foldSuggestion(diag, next *ast.CallExpr, options []string) string {
	folded := d.extendCall(diag, options)
	d1, n1 := d.nodeSpan(diag), d.nodeSpan(next)
	if d1.start < n1.start {
		between := strings.TrimRight(d.src[d1.end:n1.start], " \t\r\n")
		return "replace " + d.src[d1.start:n1.end] + " with " + folded + between
	}
	between := strings.TrimLeft(d.src[n1.end:d1.start], " \t\r\n")
	return "replace " + d.src[n1.start:d1.end] + " with " + between + folded
}

// extendCall is call's source with options appended to its argument list.
func (d *recSurfaceDetector) extendCall(call *ast.CallExpr, options []string) string {
	src := d.nodeSrc(call)
	body := strings.TrimRight(strings.TrimSuffix(src, ")"), " \t\r\n")
	body = strings.TrimSuffix(body, ",")
	return body + ", " + strings.Join(options, ", ") + ")"
}

// errorReturnProblem is the rewrite for a Next call inside its own Task's
// Define callback on a path that then returns an error: the remedy rides on an
// error-severity Problem recorded right where the Next call was. Its summary is
// the one the returned error carries when that is a string literal, else the
// placeholder (which keeps the review open).
func (d *recSurfaceDetector) errorReturnProblem(call *ast.CallExpr, recv ast.Expr, options []string) (string, bool) {
	if d.doneScope == nil || !d.doneScope.insideOwnDefine(exprDottedName(recv), d.offset(call)) {
		return "", false
	}
	ret := d.laterErrorReturn(call)
	if ret == nil {
		return "", false
	}
	summary := d.errorReturnSummary(ret)
	return d.nodeSrc(recv) + ".Problem(" + summary + ", " + strings.Join(options, ", ") + ")", true
}

// laterErrorReturn is the first return after call, in call's own function,
// whose last result is not the literal nil.
func (d *recSurfaceDetector) laterErrorReturn(call *ast.CallExpr) *ast.ReturnStmt {
	body := d.enclosingFuncBody(call)
	if body == nil {
		return nil
	}
	var found *ast.ReturnStmt
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			if found == nil && n.Pos() > call.End() && returnsError(n) {
				found = n
			}
		}
		return found == nil
	})
	return found
}

func returnsError(ret *ast.ReturnStmt) bool {
	if len(ret.Results) == 0 {
		return false
	}
	return identName(ret.Results[len(ret.Results)-1]) != "nil"
}

// errorReturnSummary is the quoted literal of errors.New("...") or
// fmt.Errorf("...") when ret returns one, else the quoted placeholder.
func (d *recSurfaceDetector) errorReturnSummary(ret *ast.ReturnStmt) string {
	placeholder := `"` + remedyWarningSummary + `"`
	call, ok := ret.Results[len(ret.Results)-1].(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return placeholder
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "New" && sel.Sel.Name != "Errorf") {
		return placeholder
	}
	lit, ok := stringLit(call.Args[0])
	if !ok || strings.Contains(lit, "%") {
		return placeholder
	}
	return d.nodeSrc(call.Args[0])
}
