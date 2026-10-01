package review

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"
)

// A Finding.Suggestion is one line (review.go). An edit that spans statements
// is therefore written as `replace <left> with <right>` where every newline run
// in <left> is a single space, which matches any whitespace run in the source,
// and <right> is one line, statements separated by "; ".

var (
	newlineRun       = regexp.MustCompile(`\s*\n\s*`)
	tidyOpenParen    = strings.NewReplacer("( ", "(", ", )", ")", ",)", ")", " )", ")")
	errorCtorPackage = map[string]string{"New": "errors", "Errorf": "fmt"}
)

// replaceSuggestion renders one single-line replace edit.
func replaceSuggestion(left, right string) string {
	return "replace " + newlineRun.ReplaceAllString(left, " ") + " with " + tidyOpenParen.Replace(newlineRun.ReplaceAllString(right, " "))
}

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

// neighbor is the statement offset away from s in its block, only when nothing
// but whitespace separates the two (a comment between them belongs to one of
// them and must not be moved).
func (d *recSurfaceDetector) neighbor(s remedyStatement, offset int) ast.Stmt {
	if s.block == nil {
		return nil
	}
	i := s.index + offset
	if i < 0 || i >= len(s.block.List) {
		return nil
	}
	first, second := s.block.List[min(s.index, i)], s.block.List[max(s.index, i)]
	if strings.TrimSpace(d.src[d.stmtEnd(first):d.offset(second)]) != "" {
		return nil
	}
	return s.block.List[i]
}

func (d *recSurfaceDetector) stmtEnd(st ast.Stmt) int { return d.nodeSpan(st).end }

// adjacentDiagnostic is the Fail/Block/Problem statement directly before or
// after the Next statement on exactly the same receiver, with no spread
// argument: the only neighbors a remedy can fold into without crossing
// control flow. A preceding statement wins over a following one.
func (d *recSurfaceDetector) adjacentDiagnostic(s remedyStatement, recvSrc string) *ast.CallExpr {
	for _, offset := range []int{-1, 1} {
		if c := d.plainDiagnostic(d.neighbor(s, offset), recvSrc); c != nil {
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

// foldSuggestion is one replace edit that spans the two adjacent statements:
// the diagnostic gains the options and the Next statement disappears.
func (d *recSurfaceDetector) foldSuggestion(diag, next *ast.CallExpr, options []string) string {
	d1, n1 := d.nodeSpan(diag), d.nodeSpan(next)
	return replaceSuggestion(d.src[min(d1.start, n1.start):max(d1.end, n1.end)], d.extendCall(diag, options))
}

// extendCall is call's source with options appended to its argument list.
func (d *recSurfaceDetector) extendCall(call *ast.CallExpr, options []string) string {
	src := d.nodeSrc(call)
	body := strings.TrimRight(strings.TrimSuffix(src, ")"), " \t\r\n")
	body = strings.TrimSuffix(body, ",")
	return body + ", " + strings.Join(options, ", ") + ")"
}

// defineFailRewrite is the rewrite for a Next call in its own Task's Define
// callback that is immediately followed by `return <error that is certainly
// non-nil>`: the callback fails the Task through Fail, carrying the remedy, and
// returns nil. The summary comes only from a literal in that very return;
// otherwise it is the placeholder, which keeps the review open.
func (d *recSurfaceDetector) defineFailRewrite(call *ast.CallExpr, recv ast.Expr, stmt remedyStatement, options []string) (string, bool) {
	ret, ok := d.neighbor(stmt, 1).(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 || !d.inOwnDefineCallback(call, recv) {
		return "", false
	}
	result := ret.Results[0]
	literal, hasLiteral := d.errorCtorLiteral(result)
	if !isErrorCtor(result) && !d.guardedNonNil(stmt.block, result) {
		return "", false
	}
	args := []string{`"` + remedyWarningSummary + `"`, d.pkg + ".Detail(" + d.errorText(result) + ")"}
	if hasLiteral {
		args = []string{literal}
	}
	fail := d.nodeSrc(recv) + ".Fail(" + strings.Join(append(args, options...), ", ") + "); return nil"
	span := d.nodeSpan(call)
	return replaceSuggestion(d.src[span.start:d.nodeSpan(ret).end], fail), true
}

// inOwnDefineCallback reports whether call's innermost function is recv's own
// Define callback.
func (d *recSurfaceDetector) inOwnDefineCallback(call *ast.CallExpr, recv ast.Expr) bool {
	if d.doneScope == nil {
		return false
	}
	lit := d.enclosingFuncLit(call)
	if lit == nil {
		return false
	}
	name, span := exprDottedName(recv), d.nodeSpan(lit)
	for _, b := range d.doneScope.defineBodies {
		if b.recv == name && b.span == span {
			return true
		}
	}
	return false
}

// enclosingFuncLit is the innermost function containing n, when it is a
// function literal.
func (d *recSurfaceDetector) enclosingFuncLit(n ast.Node) *ast.FuncLit {
	var lit *ast.FuncLit
	ast.Inspect(d.file, func(x ast.Node) bool {
		var body *ast.BlockStmt
		switch f := x.(type) {
		case *ast.FuncDecl:
			body, lit = f.Body, nil
		case *ast.FuncLit:
			body = f.Body
		}
		if body != nil && body.Pos() <= n.Pos() && n.End() <= body.End() {
			lit, _ = x.(*ast.FuncLit)
		}
		return true
	})
	return lit
}

// guardedNonNil reports whether block is the body of `if X != nil` and result is
// that same identifier X, so the return certainly carries a non-nil error.
func (d *recSurfaceDetector) guardedNonNil(block *ast.BlockStmt, result ast.Expr) bool {
	name := identName(result)
	if name == "" || name == "nil" {
		return false
	}
	guarded := false
	ast.Inspect(d.file, func(n ast.Node) bool {
		stmt, ok := n.(*ast.IfStmt)
		if !ok || stmt.Body != block {
			return !guarded
		}
		cond, ok := stmt.Cond.(*ast.BinaryExpr)
		guarded = ok && cond.Op == token.NEQ && identName(cond.X) == name && identName(cond.Y) == "nil"
		return false
	})
	return guarded
}

// isErrorCtor is errors.New(...) or fmt.Errorf(...): never nil.
func isErrorCtor(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && errorCtorPackage[sel.Sel.Name] == identName(sel.X)
}

// errorCtorLiteral is the quoted summary of errors.New("lit") or
// fmt.Errorf("lit") when the format has no verbs.
func (d *recSurfaceDetector) errorCtorLiteral(e ast.Expr) (string, bool) {
	if !isErrorCtor(e) {
		return "", false
	}
	call := e.(*ast.CallExpr)
	if len(call.Args) != 1 {
		return "", false
	}
	text, ok := stringLit(call.Args[0])
	if !ok || strings.Contains(text, "%") {
		return "", false
	}
	return d.nodeSrc(call.Args[0]), true
}

// errorText is `<result>.Error()`, parenthesizing only what needs it.
func (d *recSurfaceDetector) errorText(result ast.Expr) string {
	src := d.nodeSrc(result)
	switch result.(type) {
	case *ast.Ident, *ast.CallExpr, *ast.SelectorExpr:
		return src + ".Error()"
	}
	return "(" + src + ").Error()"
}
