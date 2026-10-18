// Package review — API-053: a second claim-taking operation made under a context an enclosing Effect already holds.
package review

import (
	"go/ast"
	"go/token"
)

// ===== API-053: nested Evo claim acquisition. Effect holds the claim its
// spec names (Path or Resource) for its fn callback's duration only when one
// is set. File and Patch always claim the path they touch but take no
// callback, so they cannot host a nested acquisition themselves. Calling
// File, Patch, or a claim-taking Effect again with that same held ctx —
// directly, or one call away through a same-file helper handed the held ctx
// — asks for a second claim while the first is still held, which Evo
// refuses at apply time (CONTRACT.md: holding two claims at once is
// misuse); this rule catches the same shape, including the indirect case,
// before it can reach runtime.

// resourceHold is one evo.Effect(ctx, spec, fn) call whose spec statically
// holds a claim: fn's body and the name fn's own context.Context
// parameter binds to inside that body.
type resourceHold struct {
	body    *ast.BlockStmt
	ctxName string
}

// resourceHoldingCallbacks finds every evo.Effect(...) call in file whose
// spec composite literal (inline, or a same-file local variable last
// assigned one, resolved through specVars) sets a Path or Resource claim.
func resourceHoldingCallbacks(file *ast.File, pkg string, specVars map[string]*ast.CompositeLit) []resourceHold {
	var holds []resourceHold
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || calledFuncDotted(call) != pkg+".Effect" || len(call.Args) < 3 {
			return true
		}
		lit := effectSpecArg(call.Args[1], specVars)
		if lit == nil || !effectSpecHoldsClaim(lit) {
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

// effectClaimFields are the EffectSpec fields that make an Effect hold a
// claim while its callback runs: Path claims a filesystem path, Resource a
// logical name.
var effectClaimFields = []string{"Path", "Resource"}

// effectSpecHoldsClaim reports whether lit sets Path or Resource to
// anything but the empty string literal.
func effectSpecHoldsClaim(lit *ast.CompositeLit) bool {
	for _, field := range effectClaimFields {
		value := compositeLitField(lit, field)
		if value == nil {
			continue
		}
		if text, isLiteral := stringLiteralValue(value); !isLiteral || text != "" {
			return true
		}
	}
	return false
}

// detectNestedResourceAcquisition walks every claim-holding Effect
// callback and, one call away through same-file helpers it hands its held
// ctx to, flags a second evo.File, evo.Patch, or claim-taking evo.Effect
// call made with that same held context identifier.
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

// scanBlockForNestedResourceAcquisition looks for pkg.File(ctxName, ...),
// pkg.Patch(ctxName, ...), or a claim-taking pkg.Effect(ctxName, ...) call anywhere in block,
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
		if dotted != pkg+".File" && dotted != pkg+".Patch" && dotted != pkg+".Effect" {
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
			if lit == nil || !effectSpecHoldsClaim(lit) {
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
		Message:    calleeDotted + "(...) is called with a context that already holds a claim from an enclosing evo.Effect's Path or Resource; holding at most one claim at a time is what makes deadlock impossible, so a second acquisition is misuse even when the second claim is free",
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: "finish and return from the first evo.Effect/evo.File before starting a second, or claim one coarser Path or Resource name that both mutations share instead of nesting a second acquisition",
	}
}
