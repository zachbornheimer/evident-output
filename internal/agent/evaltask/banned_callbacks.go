package evaltask

import (
	"go/ast"
	"go/token"
	"slices"
)

// callbackLiterals returns the function literals passed to Define or
// Compute: the bodies evo runs for a Task or a Group build.
func callbackLiterals(file *ast.File) []*ast.FuncLit {
	var lits []*ast.FuncLit
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if method := calledMethod(call); method != methodDefine && method != methodCompute {
			return true
		}
		for _, arg := range call.Args {
			if lit, isLit := arg.(*ast.FuncLit); isLit {
				lits = append(lits, lit)
			}
		}
		return true
	})
	return lits
}

func takesContext(lit *ast.FuncLit) bool {
	for _, param := range lit.Type.Params.List {
		if sel, ok := param.Type.(*ast.SelectorExpr); ok && sel.Sel.Name == "Context" {
			return true
		}
	}
	return false
}

func hasLoopInTaskCallback(file *ast.File) bool {
	for _, lit := range callbackLiterals(file) {
		if takesContext(lit) && containsLoop(lit.Body) {
			return true
		}
	}
	return false
}

func containsLoop(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			found = true
		}
		return !found
	})
	return found
}

func hasDeclarationInCallback(file *ast.File) bool {
	for _, lit := range callbackLiterals(file) {
		if takesContext(lit) && declaresTopology(lit.Body) {
			return true
		}
	}
	return false
}

func declaresTopology(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			switch calledMethod(call) {
			case methodTask, methodGroup, methodSequence:
				found = true
			}
		}
		return !found
	})
	return found
}

func hasOuterHandoff(file *ast.File) bool {
	return slices.ContainsFunc(callbackLiterals(file), assignsOutside)
}

// assignsOutside reports whether lit assigns to a plain variable it did not
// declare: state handed from one callback to another by side channel.
func assignsOutside(lit *ast.FuncLit) bool {
	declared := declaredIn(lit)
	found := false
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		for _, target := range assignedIdents(n) {
			if target.Name != "_" && !declared[target.Name] {
				found = true
			}
		}
		return !found
	})
	return found
}

func declaredIn(lit *ast.FuncLit) map[string]bool {
	declared := map[string]bool{}
	for _, param := range lit.Type.Params.List {
		for _, name := range param.Names {
			declared[name.Name] = true
		}
	}
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			if node.Tok == token.DEFINE {
				markIdents(declared, node.Lhs)
			}
		case *ast.RangeStmt:
			markIdents(declared, []ast.Expr{node.Key, node.Value})
		case *ast.ValueSpec:
			for _, name := range node.Names {
				declared[name.Name] = true
			}
		}
		return true
	})
	return declared
}

func markIdents(set map[string]bool, exprs []ast.Expr) {
	for _, expr := range exprs {
		if ident, ok := expr.(*ast.Ident); ok {
			set[ident.Name] = true
		}
	}
}

// assignedIdents returns the plain variables n writes (=, +=, ++), not
// declarations.
func assignedIdents(n ast.Node) []*ast.Ident {
	var targets []ast.Expr
	switch node := n.(type) {
	case *ast.AssignStmt:
		if node.Tok != token.DEFINE {
			targets = node.Lhs
		}
	case *ast.IncDecStmt:
		targets = []ast.Expr{node.X}
	}
	var idents []*ast.Ident
	for _, target := range targets {
		if ident, ok := target.(*ast.Ident); ok {
			idents = append(idents, ident)
		}
	}
	return idents
}

// hasRedundantSequenceAfter reports an After on a Sequence child whose
// predecessor is a sibling in that same Sequence: declaration order already
// says it.
func hasRedundantSequenceAfter(file *ast.File) bool {
	sequences, container := sequenceBindings(file)
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || calledMethod(call) != methodAfter {
			return !found
		}
		owner := sequenceOwningTask(call, sequences)
		for _, arg := range call.Args {
			if ident, isIdent := arg.(*ast.Ident); isIdent && owner != "" && container[ident.Name] == owner {
				found = true
			}
		}
		return !found
	})
	return found
}

// sequenceOwningTask names the Sequence variable when call is
// seq.Task(...).After(...), else "".
func sequenceOwningTask(after *ast.CallExpr, sequences map[string]bool) string {
	sel, ok := after.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	return taskReceiver(sel.X, sequences)
}

func taskReceiver(expr ast.Expr, sequences map[string]bool) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok || calledMethod(call) != methodTask {
		return ""
	}
	recv, ok := call.Fun.(*ast.SelectorExpr).X.(*ast.Ident)
	if ok && sequences[recv.Name] {
		return recv.Name
	}
	return ""
}

// sequenceBindings finds variables holding a Sequence, and which Sequence
// each variable's Task was declared in.
func sequenceBindings(file *ast.File) (map[string]bool, map[string]string) {
	sequences := map[string]bool{}
	container := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		name, isIdent := assign.Lhs[0].(*ast.Ident)
		if !isIdent {
			return true
		}
		if call, isCall := assign.Rhs[0].(*ast.CallExpr); isCall && calledMethod(call) == methodSequence {
			sequences[name.Name] = true
			return true
		}
		ast.Inspect(assign.Rhs[0], func(inner ast.Node) bool {
			if owner := taskReceiver(asExpr(inner), sequences); owner != "" {
				container[name.Name] = owner
			}
			return true
		})
		return true
	})
	return sequences, container
}

func asExpr(n ast.Node) ast.Expr {
	expr, _ := n.(ast.Expr)
	return expr
}
