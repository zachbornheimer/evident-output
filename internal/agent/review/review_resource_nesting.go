// Package review — API-053: a second resource-claiming operation made under a context an enclosing Effect already holds.
package review

import (
	"go/ast"
	"go/token"
)

// ===== API-053: nested Evo resource acquisition (ZYS-933/ZYS-840). Effect
// holds spec.Resource for its fn callback's duration only when spec.Resource
// is set (File always claims its own path automatically, but takes no
// callback, so it cannot itself host a nested acquisition). Calling File or
// a Resource-claiming Effect again with that same held ctx — directly, or
// one call away through a same-file helper handed the held ctx — asks for a
// second resource while the first is still held. The runtime check
// (evo.ErrNestedResourceAcquisition) rejects it deterministically at apply
// time; this rule catches the same shape, including the indirect case,
// before it can reach runtime.

// resourceHold is one evo.Effect(ctx, spec, fn) call whose spec statically
// claims a Resource: fn's body and the name fn's own context.Context
// parameter binds to inside that body.
type resourceHold struct {
	body    *ast.BlockStmt
	ctxName string
}

// resourceHoldingCallbacks finds every evo.Effect(...) call in file whose
// spec composite literal (inline, or a same-file local variable last
// assigned one, resolved through specVars) sets a non-nil Resource field.
func resourceHoldingCallbacks(file *ast.File, pkg string, specVars map[string]*ast.CompositeLit) []resourceHold {
	var holds []resourceHold
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || calledFuncDotted(call) != pkg+".Effect" || len(call.Args) < 3 {
			return true
		}
		lit := effectSpecArg(call.Args[1], specVars)
		if lit == nil || !compositeLitClaimsResource(lit) {
			return true
		}
		fl, ok := call.Args[2].(*ast.FuncLit)
		if !ok || fl.Type.Params == nil || len(fl.Type.Params.List) == 0 {
			return true
		}
		names := fl.Type.Params.List[0].Names
		if len(names) == 0 {
			return true
		}
		holds = append(holds, resourceHold{body: fl.Body, ctxName: names[0].Name})
		return true
	})
	return holds
}

// effectSpecArg resolves arg (the second argument to evo.Effect) to its
// pkg.EffectSpec composite literal, whether written inline or through a
// same-file local variable resolved via specVars.
func effectSpecArg(arg ast.Expr, specVars map[string]*ast.CompositeLit) *ast.CompositeLit {
	if lit, ok := arg.(*ast.CompositeLit); ok {
		return lit
	}
	if id, ok := arg.(*ast.Ident); ok {
		return specVars[id.Name]
	}
	return nil
}

// effectSpecCompositeLits maps a local variable name to the pkg.EffectSpec
// composite literal it was last assigned from, same-file only — the same
// "obvious static case" scope every helper-following MCP rule in this file
// uses (no cross-package resolution, no type checking).
func effectSpecCompositeLits(file *ast.File, pkg string) map[string]*ast.CompositeLit {
	out := map[string]*ast.CompositeLit{}
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != len(assign.Rhs) {
			return true
		}
		for i, rhs := range assign.Rhs {
			lit, ok := rhs.(*ast.CompositeLit)
			if !ok {
				continue
			}
			sel, ok := lit.Type.(*ast.SelectorExpr)
			if !ok || exprDottedName(sel.X) != pkg || sel.Sel.Name != "EffectSpec" {
				continue
			}
			if id, ok := assign.Lhs[i].(*ast.Ident); ok {
				out[id.Name] = lit
			}
		}
		return true
	})
	return out
}

// compositeLitClaimsResource reports whether lit sets a Resource field to
// a non-nil expression.
func compositeLitClaimsResource(lit *ast.CompositeLit) bool {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Resource" {
			continue
		}
		id, ok := kv.Value.(*ast.Ident)
		return !ok || id.Name != "nil"
	}
	return false
}

// detectNestedResourceAcquisition walks every Resource-claiming Effect
// callback and, one call away through same-file helpers it hands its held
// ctx to, flags a second evo.File or Resource-claiming evo.Effect call made
// with that same held context identifier.
func detectNestedResourceAcquisition(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" {
		return nil
	}
	funcs := map[string]*ast.FuncDecl{}
	ast.Inspect(file, func(n ast.Node) bool {
		if fd, ok := n.(*ast.FuncDecl); ok && fd.Body != nil {
			funcs[fd.Name.Name] = fd
		}
		return true
	})
	specVars := effectSpecCompositeLits(file, pkg)

	var findings []Finding
	for _, hold := range resourceHoldingCallbacks(file, pkg, specVars) {
		visited := map[*ast.BlockStmt]bool{}
		var visit func(block *ast.BlockStmt, ctxName string, depth int)
		visit = func(block *ast.BlockStmt, ctxName string, depth int) {
			if block == nil || visited[block] || depth > 2 || ctxName == "" || ctxName == "_" {
				return
			}
			visited[block] = true
			findings = append(findings, scanBlockForNestedResourceAcquisition(filename, block, fset, pkg, ctxName, specVars)...)
			ast.Inspect(block, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				fd, ok := funcs[calledFuncName(call)]
				if !ok {
					return true
				}
				visit(fd.Body, forwardedParamName(fd, call, ctxName), depth+1)
				return true
			})
		}
		visit(hold.body, hold.ctxName, 0)
	}
	return findings
}

// scanBlockForNestedResourceAcquisition looks for pkg.File(ctxName, ...) or
// a Resource-claiming pkg.Effect(ctxName, ...) call anywhere in block,
// never descending into a nested FuncLit (that closure's own resource
// holds, if any, are tracked as their own resourceHold instead).
func scanBlockForNestedResourceAcquisition(filename string, block *ast.BlockStmt, fset *token.FileSet, pkg, ctxName string, specVars map[string]*ast.CompositeLit) []Finding {
	var findings []Finding
	ast.Inspect(block, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		dotted := calledFuncDotted(call)
		if dotted != pkg+".File" && dotted != pkg+".Effect" {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}
		id, ok := call.Args[0].(*ast.Ident)
		if !ok || id.Name != ctxName {
			return true
		}
		if dotted == pkg+".Effect" {
			if len(call.Args) < 2 {
				return true
			}
			lit := effectSpecArg(call.Args[1], specVars)
			if lit == nil || !compositeLitClaimsResource(lit) {
				return true
			}
		}
		pos := fset.Position(call.Pos())
		findings = append(findings, nestedResourceAcquisitionFinding(filename, pos, dotted))
		return true
	})
	return findings
}

// forwardedParamName reports the name fd's parameter list binds to at
// call's matching argument position for ctxName — the same same-file,
// name-based "one call away" resolution every helper-following MCP rule in
// this file uses (no cross-package resolution, no type checking).
func forwardedParamName(fd *ast.FuncDecl, call *ast.CallExpr, ctxName string) string {
	if fd.Type.Params == nil {
		return ""
	}
	var params []string
	for _, field := range fd.Type.Params.List {
		if len(field.Names) == 0 {
			params = append(params, "")
			continue
		}
		for _, n := range field.Names {
			params = append(params, n.Name)
		}
	}
	for i, arg := range call.Args {
		id, ok := arg.(*ast.Ident)
		if !ok || id.Name != ctxName {
			continue
		}
		if i < len(params) {
			return params[i]
		}
	}
	return ""
}

func nestedResourceAcquisitionFinding(filename string, pos token.Position, calleeDotted string) Finding {
	return Finding{
		RuleID:     "API-053",
		Message:    calleeDotted + "(...) is called with a context that already holds a Resource from an enclosing evo.Effect's spec.Resource claim; holding at most one Resource at a time is what makes deadlock impossible, so a second acquisition is misuse even when the second resource is free",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: "finish and return from the first evo.Effect/evo.File before starting a second, or claim one coarser Resource (e.g. evo.FSResource covering both paths) that both mutations share instead of nesting a second acquisition",
	}
}
