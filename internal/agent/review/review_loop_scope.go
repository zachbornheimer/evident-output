package review

import (
	"go/ast"
	"go/token"
)

// nodeSpan is one node's source range.
type nodeSpan struct{ pos, end token.Pos }

func spanOf(n ast.Node) nodeSpan { return nodeSpan{n.Pos(), n.End()} }

func (s nodeSpan) contains(n nodeSpan) bool { return s.pos <= n.pos && n.end <= s.end }

// loopScope is one loop a call sits in: its label, the names its header
// and body bind (a receiver bound there is a fresh value each iteration),
// and the blocks of its body that end by transferring control. Nested
// loops and function literals are their own scopes.
type loopScope struct {
	label string
	span  nodeSpan
	bound map[string]bool
	ends  []blockEnd
}

// blockEnd is a statement block whose last statement transfers control:
// what the block holds runs once, then control goes where last says.
type blockEnd struct {
	span     nodeSpan
	last     ast.Stmt
	inSwitch bool // an unlabeled break here leaves only a switch or select
}

// newLoopScope scopes loop (a *ast.ForStmt or *ast.RangeStmt) under label.
func newLoopScope(loop ast.Stmt, label string) *loopScope {
	scope := &loopScope{label: label, span: spanOf(loop)}
	var body *ast.BlockStmt
	switch l := loop.(type) {
	case *ast.ForStmt:
		body, scope.bound = l.Body, boundNames(l.Init)
	case *ast.RangeStmt:
		body, scope.bound = l.Body, boundNames(l)
	}
	for name := range boundNames(body) {
		scope.bound[name] = true
	}
	scope.collectEnds(body, false)
	return scope
}

// collectEnds records every block under n that ends in a transfer.
func (l *loopScope) collectEnds(n ast.Node, inSwitch bool) {
	ast.Inspect(n, func(child ast.Node) bool {
		if child != n {
			switch child.(type) {
			case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
				return false
			case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
				l.collectEnds(child, true)
				return false
			}
		}
		if last := lastStmt(child); isTransfer(last) {
			l.ends = append(l.ends, blockEnd{span: spanOf(child), last: last, inSwitch: inSwitch})
		}
		return true
	})
}

// endAround is the innermost transferring block of this loop's own body
// that holds n, if any.
func (l *loopScope) endAround(n ast.Node) (blockEnd, bool) {
	var best blockEnd
	found := false
	for _, e := range l.ends {
		if e.span.contains(spanOf(n)) && (!found || best.span.contains(e.span)) {
			best, found = e, true
		}
	}
	return best, found
}

// lastStmt is the final statement of a block or case body, or nil.
func lastStmt(n ast.Node) ast.Stmt {
	var list []ast.Stmt
	switch b := n.(type) {
	case *ast.BlockStmt:
		list = b.List
	case *ast.CaseClause:
		list = b.Body
	case *ast.CommClause:
		list = b.Body
	}
	if len(list) == 0 {
		return nil
	}
	return list[len(list)-1]
}

func isTransfer(stmt ast.Stmt) bool {
	switch s := stmt.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.BranchStmt:
		return s.Tok != token.FALLTHROUGH
	}
	return false
}

// labelSpans is where each label in one function body sits. A function
// literal has labels of its own and is not entered.
func labelSpans(body *ast.BlockStmt) map[string]nodeSpan {
	spans := map[string]nodeSpan{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.LabeledStmt:
			spans[s.Label.Name] = spanOf(s)
		}
		return true
	})
	return spans
}
