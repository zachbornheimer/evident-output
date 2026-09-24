package review

import (
	"go/ast"
	"go/token"
)

// loopExits is which parts of one loop body run at most once per loop:
// every statement block whose last statement leaves the loop (return,
// goto, or a break that targets the loop rather than an enclosing switch
// or select). Whatever such a block holds runs once, then the loop ends.
type loopExits struct {
	spans []nodeSpan
}

// nodeSpan is one node's source range.
type nodeSpan struct{ pos, end token.Pos }

// newLoopExits collects body's leaving blocks. Nested loops and function
// literals are their own scopes and are not entered.
func newLoopExits(body *ast.BlockStmt) loopExits {
	var exits loopExits
	var walk func(n ast.Node, inSwitch bool)
	walk = func(n ast.Node, inSwitch bool) {
		ast.Inspect(n, func(child ast.Node) bool {
			if child == n {
				exits.addIfLeaving(child, inSwitch)
				return true
			}
			switch child.(type) {
			case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
				return false
			case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
				walk(child, true)
				return false
			}
			exits.addIfLeaving(child, inSwitch)
			return true
		})
	}
	walk(body, false)
	return exits
}

// addIfLeaving records n when it is a statement list ending in a loop exit.
func (e *loopExits) addIfLeaving(n ast.Node, inSwitch bool) {
	if leavesLoop(lastStmt(n), inSwitch) {
		e.spans = append(e.spans, nodeSpan{n.Pos(), n.End()})
	}
}

// RunsOnce reports whether n sits in a block that leaves the loop.
func (e loopExits) RunsOnce(n ast.Node) bool {
	for _, s := range e.spans {
		if s.pos <= n.Pos() && n.End() <= s.end {
			return true
		}
	}
	return false
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

// leavesLoop reports whether stmt ends the enclosing loop. An unlabeled
// break inside a switch or select leaves only that switch or select.
func leavesLoop(stmt ast.Stmt, inSwitch bool) bool {
	switch s := stmt.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.BranchStmt:
		switch s.Tok {
		case token.GOTO:
			return true
		case token.BREAK:
			return s.Label != nil || !inSwitch
		}
	}
	return false
}
