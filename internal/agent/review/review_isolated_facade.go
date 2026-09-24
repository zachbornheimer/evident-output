// Package review — API-062 (ZYS-946, spec §53): a package-level evo
// declaration (evo.Task, evo.Group, evo.Sequence, evo.Fact, evo.Warn,
// evo.Print*, evo.Confirm) inside the Run callback of an
// Isolated Output. Package-level calls always reach the package default,
// never the Output being run, so under an HTTP server every concurrent
// request would share that one default's state while its own document
// came back empty. The fix is the Output-bound spelling: out.Task(...),
// or pass the *evo.Output into the shared model function.
//
// Detection is per-function-body and structural: an identifier assigned
// from evo.Init(evo.Config{..., Isolated: true, ...}), then called as
// ident.Run(ctx, func literal); every package-level declaration call
// inside that literal (including nested Define callbacks) is flagged. A
// model function declared elsewhere is not followed across the call.
package review

import (
	"go/ast"
	"go/token"
)

// packageDeclarationFacades are the package-level evo functions that act
// on the package default Output and have an Output-bound method twin.
var packageDeclarationFacades = map[string]bool{
	"Task": true, "Group": true, "Sequence": true, "Fact": true, "Warn": true,
	"Print": true, "Printf": true, "Println": true, "Confirm": true,
}

// detectPackageFacadeInIsolatedRun is API-062.
func detectPackageFacadeInIsolatedRun(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" {
		return nil
	}
	var findings []Finding
	forEachFuncBody(file, func(body *ast.BlockStmt) {
		isolated := isolatedOutputVars(body, pkg)
		if len(isolated) == 0 {
			return
		}
		for _, run := range isolatedRunCallbacks(body, isolated) {
			ast.Inspect(run.callback, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if fn, ok := packageFacadeCall(call, pkg); ok {
					findings = append(findings, packageFacadeInIsolatedRunFinding(filename, fset.Position(call.Pos()), pkg, fn, run.output))
				}
				return true
			})
		}
	})
	return findings
}

// isolatedRun is one out.Run(ctx, func...) call on an Isolated Output.
type isolatedRun struct {
	output   string
	callback *ast.FuncLit
}

// isolatedOutputVars names every variable body assigns from
// evo.Init(evo.Config{Isolated: true, ...}).
func isolatedOutputVars(body *ast.BlockStmt, pkg string) map[string]bool {
	vars := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		stmt, ok := n.(*ast.AssignStmt)
		if !ok || len(stmt.Lhs) != 1 || len(stmt.Rhs) != 1 {
			return true
		}
		id, ok := stmt.Lhs[0].(*ast.Ident)
		call, isCall := stmt.Rhs[0].(*ast.CallExpr)
		if ok && isCall && calledFuncDotted(call) == pkg+".Init" && configIsIsolated(call.Args) {
			vars[id.Name] = true
		}
		return true
	})
	return vars
}

// configIsIsolated reports whether Init's Config literal sets Isolated: true.
func configIsIsolated(args []ast.Expr) bool {
	if len(args) == 0 {
		return false
	}
	lit, ok := args[0].(*ast.CompositeLit)
	if !ok {
		return false
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, keyOK := kv.Key.(*ast.Ident)
		val, valOK := kv.Value.(*ast.Ident)
		if keyOK && valOK && key.Name == "Isolated" && val.Name == "true" {
			return true
		}
	}
	return false
}

// isolatedRunCallbacks finds each isolated.Run(ctx, func literal) in body.
func isolatedRunCallbacks(body *ast.BlockStmt, isolated map[string]bool) []isolatedRun {
	var runs []isolatedRun
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Run" {
			return true
		}
		recv, ok := sel.X.(*ast.Ident)
		callback, isLit := call.Args[1].(*ast.FuncLit)
		if ok && isLit && isolated[recv.Name] {
			runs = append(runs, isolatedRun{output: recv.Name, callback: callback})
		}
		return true
	})
	return runs
}

// packageFacadeCall reports the facade name when call is pkg.<facade>(...).
func packageFacadeCall(call *ast.CallExpr, pkg string) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !isEvoIdent(sel.X, pkg) || !packageDeclarationFacades[sel.Sel.Name] {
		return "", false
	}
	return sel.Sel.Name, true
}

func packageFacadeInIsolatedRunFinding(filename string, pos token.Position, pkg, fn, output string) Finding {
	return Finding{
		RuleID:   "API-062",
		Severity: "error",
		Message: pkg + "." + fn + " inside " + output + ".Run declares on the package default, not on the Isolated Output being run — " +
			"concurrent runs share that default and this run's own document omits the work",
		File:            filename,
		Line:            pos.Line,
		Column:          pos.Column,
		Suggestion:      "call " + output + "." + fn + "(...) instead, or pass " + output + " (*" + pkg + ".Output) into the shared model function so the CLI and HTTP paths declare on whichever Output drives them",
		RequiredVersion: dialectOneZero,
	}
}
