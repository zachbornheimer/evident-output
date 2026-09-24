// Package review — API-049: a Define callback that discards its scheduler-provided context.
package review

import (
	"go/ast"
	"go/token"
)

// ===== API-049: task.Define(func(context.Context) error { ... }) discards
// its scheduler-provided context parameter (unnamed, or named something other
// than the outer "ctx" it shadows) while the body still calls cancellable
// work with the captured outer "ctx" — the row cancels correctly but the
// work it names never observes that cancellation (ZYS-938 / evo-1.x Decisions
// 2026-09-23: "the Define context is authoritative for task
// cancellation/lifecycle").

// defineCallbackContextParamName reports the Define callback's single
// context.Context parameter name ("" for an unnamed or blank-identifier
// parameter) and whether the signature matches func(context.Context) error
// at all.
func defineCallbackContextParamName(ft *ast.FuncType) (string, bool) {
	if ft.Params == nil || len(ft.Params.List) != 1 {
		return "", false
	}
	field := ft.Params.List[0]
	sel, ok := field.Type.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Context" || exprDottedName(sel.X) != "context" {
		return "", false
	}
	if len(field.Names) == 0 {
		return "", true
	}
	name := field.Names[0].Name
	if name == "_" {
		return "", true
	}
	return name, true
}

// bodyCallsWithCapturedIdent reports whether block contains a call passing
// ident as an argument that still resolves to the callback's captured outer
// variable. A local re-declaration of ident (ctx := ..., var ctx ...) in an
// earlier statement of the same or an enclosing block shadows the outer
// variable from that point on — Go's scoping rules start the new binding's
// scope right after the declaring statement — so a call after the shadow
// that passes ident reaches the local, not the ctx Define's callback
// discarded, and is not this rule's shape.
func bodyCallsWithCapturedIdent(block *ast.BlockStmt, ident string) bool {
	return stmtsCallWithCapturedIdent(block.List, ident, false)
}

// stmtsCallWithCapturedIdent walks stmts in declaration order, threading
// whether ident has already been locally shadowed by a preceding statement
// in this same block into every statement (and, for block-bearing
// statements, into their nested blocks) that follows.
func stmtsCallWithCapturedIdent(stmts []ast.Stmt, ident string, shadowed bool) bool {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.BlockStmt:
			if stmtsCallWithCapturedIdent(s.List, ident, shadowed) {
				return true
			}
			continue
		case *ast.IfStmt:
			if ifStmtCallsWithCapturedIdent(s, ident, shadowed) {
				return true
			}
			continue
		case *ast.ForStmt:
			if s.Body != nil && stmtsCallWithCapturedIdent(s.Body.List, ident, shadowed) {
				return true
			}
			continue
		case *ast.RangeStmt:
			if s.Body != nil && stmtsCallWithCapturedIdent(s.Body.List, ident, shadowed) {
				return true
			}
			continue
		case *ast.SwitchStmt:
			if caseClausesCallWithCapturedIdent(s.Body, ident, shadowed) {
				return true
			}
			continue
		case *ast.TypeSwitchStmt:
			if caseClausesCallWithCapturedIdent(s.Body, ident, shadowed) {
				return true
			}
			continue
		case *ast.SelectStmt:
			if commClausesCallWithCapturedIdent(s.Body, ident, shadowed) {
				return true
			}
			continue
		case *ast.LabeledStmt:
			if stmtsCallWithCapturedIdent([]ast.Stmt{s.Stmt}, ident, shadowed) {
				return true
			}
			continue
		}
		if !shadowed && stmtLeafCallsWithIdent(stmt, ident) {
			return true
		}
		if stmtDeclaresLocalIdent(stmt, ident) {
			shadowed = true
		}
	}
	return false
}

func ifStmtCallsWithCapturedIdent(s *ast.IfStmt, ident string, shadowed bool) bool {
	if s.Init != nil {
		if !shadowed && stmtLeafCallsWithIdent(s.Init, ident) {
			return true
		}
		if stmtDeclaresLocalIdent(s.Init, ident) {
			shadowed = true
		}
	}
	if s.Body != nil && stmtsCallWithCapturedIdent(s.Body.List, ident, shadowed) {
		return true
	}
	if s.Else != nil {
		return stmtsCallWithCapturedIdent([]ast.Stmt{s.Else}, ident, shadowed)
	}
	return false
}

func caseClausesCallWithCapturedIdent(body *ast.BlockStmt, ident string, shadowed bool) bool {
	if body == nil {
		return false
	}
	for _, stmt := range body.List {
		cc, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		if stmtsCallWithCapturedIdent(cc.Body, ident, shadowed) {
			return true
		}
	}
	return false
}

func commClausesCallWithCapturedIdent(body *ast.BlockStmt, ident string, shadowed bool) bool {
	if body == nil {
		return false
	}
	for _, stmt := range body.List {
		cc, ok := stmt.(*ast.CommClause)
		if !ok {
			continue
		}
		if stmtsCallWithCapturedIdent(cc.Body, ident, shadowed) {
			return true
		}
	}
	return false
}

// stmtDeclaresLocalIdent reports whether stmt is a `ident := ...` or
// `var ident ...` declaration — the two shapes that start a new binding for
// ident, shadowing any outer variable of the same name from here on.
func stmtDeclaresLocalIdent(stmt ast.Stmt, ident string) bool {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		if s.Tok != token.DEFINE {
			return false
		}
		for _, lhs := range s.Lhs {
			if id, ok := lhs.(*ast.Ident); ok && id.Name == ident {
				return true
			}
		}
	case *ast.DeclStmt:
		gen, ok := s.Decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			return false
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if name.Name == ident {
					return true
				}
			}
		}
	}
	return false
}

// stmtLeafCallsWithIdent reports a call passing ident as an argument
// anywhere within stmt, for a statement kind with no nested block of its
// own sibling statements to thread shadowing through (an ExprStmt, a
// ReturnStmt, ... — nested FuncLit bodies are still descended into, matching
// this rule's original scope of also catching a closure that re-discards
// the same captured ctx).
func stmtLeafCallsWithIdent(stmt ast.Stmt, ident string) bool {
	found := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		for _, arg := range call.Args {
			if id, ok := arg.(*ast.Ident); ok && id.Name == ident {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// localDefineReceiverTypes collects every type declared in this file that
// has its own Define(fn func(context.Context) error) method — a shape a
// consumer file may legitimately declare for reasons unrelated to evo (a
// validator, a config builder, a test helper). A call to one of these
// types' Define is not evo's TaskHandle.Define and must not be attributed
// to the scheduler (ZYS-938: proven false positive on such a type sharing a
// file with real evo usage).
func localDefineReceiverTypes(file *ast.File) map[string]bool {
	types := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 || fn.Name.Name != "Define" {
			continue
		}
		if !isDefineCtxCallbackMethod(fn.Type) {
			continue
		}
		if name := typeNameOf(fn.Recv.List[0].Type); name != "" {
			types[name] = true
		}
	}
	return types
}

// isDefineCtxCallbackMethod reports whether ft is a method signature
// shaped like Define(fn func(context.Context) error) — a single parameter
// that is itself a func(context.Context) error. This is the outer method
// signature (its one parameter names the callback), unlike
// defineCallbackContextParamName, which inspects that inner callback's own
// signature.
func isDefineCtxCallbackMethod(ft *ast.FuncType) bool {
	if ft.Params == nil || len(ft.Params.List) != 1 {
		return false
	}
	fnType, ok := ft.Params.List[0].Type.(*ast.FuncType)
	if !ok {
		return false
	}
	_, isCtxCallback := defineCallbackContextParamName(fnType)
	return isCtxCallback
}

// typeNameOf returns a bare or pointer type expression's identifier name
// ("Validator" for both Validator and *Validator), or "" for anything else
// (a selector into another package, a generic instantiation, ...).
func typeNameOf(e ast.Expr) string {
	if star, ok := e.(*ast.StarExpr); ok {
		e = star.X
	}
	id, ok := e.(*ast.Ident)
	if !ok {
		return ""
	}
	return id.Name
}

// identDeclaredTypeName is a best-effort, file-wide scan for a var/:=
// declaration's or a parameter's type for name. It is not scoped to the
// enclosing function — a coarse approximation — but is enough to keep a
// receiver known to be one of localDefineReceiverTypes' non-evo types from
// being mistaken for evo's TaskHandle; it returns "" (no exclusion) for
// anything it cannot resolve, so it only ever narrows this rule, never
// widens it.
func identDeclaredTypeName(file *ast.File, name string) string {
	result := ""
	ast.Inspect(file, func(n ast.Node) bool {
		if result != "" {
			return false
		}
		switch s := n.(type) {
		case *ast.AssignStmt:
			if s.Tok != token.DEFINE || len(s.Lhs) != len(s.Rhs) {
				return true
			}
			for i, lhs := range s.Lhs {
				id, ok := lhs.(*ast.Ident)
				if !ok || id.Name != name {
					continue
				}
				if t := assignedTypeName(s.Rhs[i]); t != "" {
					result = t
				}
			}
		case *ast.ValueSpec:
			for _, id := range s.Names {
				if id.Name == name && s.Type != nil {
					result = typeNameOf(s.Type)
				}
			}
		case *ast.Field:
			for _, id := range s.Names {
				if id.Name == name {
					result = typeNameOf(s.Type)
				}
			}
		}
		return true
	})
	return result
}

// assignedTypeName names the composite-literal type an expression
// constructs (T{...} or &T{...}), or "" when it constructs anything else.
func assignedTypeName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.CompositeLit:
		return typeNameOf(v.Type)
	case *ast.UnaryExpr:
		if v.Op == token.AND {
			return assignedTypeName(v.X)
		}
	}
	return ""
}

func detectDefineDiscardsSchedulerContext(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	localTypes := localDefineReceiverTypes(file)
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Define" || len(call.Args) != 1 {
			return true
		}
		if len(localTypes) > 0 {
			if recv, ok := sel.X.(*ast.Ident); ok {
				if t := identDeclaredTypeName(file, recv.Name); t != "" && localTypes[t] {
					return true
				}
			}
		}
		fl, ok := call.Args[0].(*ast.FuncLit)
		if !ok {
			return true
		}
		paramName, isCtxCallback := defineCallbackContextParamName(fl.Type)
		if !isCtxCallback || paramName == "ctx" {
			return true
		}
		if fl.Body == nil || !bodyCallsWithCapturedIdent(fl.Body, "ctx") {
			return true
		}
		pos := fset.Position(call.Pos())
		findings = append(findings, Finding{
			RuleID:     "API-049",
			Severity:   "error",
			Message:    "Define's callback discards its scheduler-provided context and calls cancellable work with a captured outer ctx instead; the scheduler's cancellation never reaches that work",
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Suggestion: "name the callback parameter ctx (func(ctx context.Context) error) and pass that ctx into the work instead of the captured outer variable",
		})
		return true
	})
	return findings
}
