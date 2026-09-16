// Package review — spec §57 EVO-EVIDENCE-001/EVO-VERIFY-001/EVO-DRYRUN-001:
// the three shapes where a callback that promises read-only observation or
// dry-run safety instead raw-calls a side effect Evo's runtime cannot see
// or intercept.
package review

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// evoDagEvidenceRequiredVersion is the minimum evident-output release whose
// public API supports every fix these detectors suggest (Verify, evo.File,
// evo.Exec, Sequence/After) — 1.0.0 also removes Task.Each and MainWith, so
// no earlier release's dialect matches this guidance (spec §57/§62). Shares
// dialectOneZero, the same cutoff GoSourceAt gates these detectors on, so
// the Suggestion's version and the firing version can never drift apart.
const evoDagEvidenceRequiredVersion = dialectOneZero

// evoRawMutationCallNames are side effects Evo's runtime cannot intercept —
// a Define/Verify/Evidence callback that promises dry-run safety or
// read-only observation must route mutation through evo.File, evo.Exec, or
// a typed mutation verb instead of calling one of these directly (spec
// §32.2, §57 EVO-DRYRUN-001).
var evoRawMutationCallNames = map[string]bool{
	"os.WriteFile": true, "os.Remove": true, "os.RemoveAll": true,
	"os.Mkdir": true, "os.MkdirAll": true, "os.Rename": true,
	"os.Chmod": true, "os.Truncate": true, "os.Symlink": true,
	"os.Create": true, "ioutil.WriteFile": true, "exec.Command": true,
}

// evoSQLMutationReceivers is the narrow, deliberately small set of receiver
// names this detector recognizes as a database handle — it only fires
// Exec/ExecContext on one of these, never on an arbitrary type that happens
// to expose the same method name.
var evoSQLMutationReceivers = map[string]bool{
	"db": true, "tx": true, "conn": true, "database": true, "sqldb": true,
}

// firstRawMutationCall reports the first evoRawMutationCallNames call, or an
// Exec/ExecContext call on an evoSQLMutationReceivers handle, reachable
// anywhere inside node (including nested closures) — the "obvious mutation"
// shape spec §57 asks EVO-EVIDENCE-001/EVO-VERIFY-001/EVO-DRYRUN-001 to flag.
func firstRawMutationCall(node ast.Node) (pos token.Pos, name string, found bool) {
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		if dotted := calledFuncDotted(call); evoRawMutationCallNames[dotted] {
			pos, name, found = call.Pos(), dotted, true
			return false
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if !isSel || (sel.Sel.Name != "Exec" && sel.Sel.Name != "ExecContext") {
			return true
		}
		if recv := strings.ToLower(exprDottedName(sel.X)); evoSQLMutationReceivers[recv] {
			pos, name, found = call.Pos(), exprDottedName(sel.X)+"."+sel.Sel.Name, true
			return false
		}
		return true
	})
	return pos, name, found
}

// funcLitArgAt returns call's argN as a *ast.FuncLit when the call is a
// method (sel.X is the receiver) with exactly argCount arguments, else
// ok=false. disproven is evoDisprovenVars(file) — pass the same set the
// caller already computed once per file.
func funcLitArgAt(call *ast.CallExpr, method string, argCount, argN int, disproven map[string]bool) (*ast.SelectorExpr, *ast.FuncLit, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != method || len(call.Args) != argCount || !isLikelyEvoTaskReceiver(sel.X, disproven) {
		return nil, nil, false
	}
	fl, ok := call.Args[argN].(*ast.FuncLit)
	if !ok {
		return nil, nil, false
	}
	return sel, fl, true
}

// isLikelyEvoTaskReceiver is isLikelyEvoReceiver's known-package check
// without the Start-only ambiguous-name exclusion (see isLikelyEvoReceiver's
// doc comment) — used by the deterministic EVO-EVIDENCE-001/VERIFY-001/
// DRYRUN-001/DAG-002/DAG-003 detectors. Their gating method names (Evidence,
// Verify, Define, After) are Evo-specific enough that excluding a variable
// by name alone (e.g. "process", "child") would silently miss a real
// *evo.TaskHandle sharing that name; disproven instead names identifiers
// this file can structurally prove are not Evo (spec §57).
func isLikelyEvoTaskReceiver(x ast.Expr, disproven map[string]bool) bool {
	switch v := x.(type) {
	case *ast.Ident:
		if knownNonEvoPackages[v.Name] || disproven[v.Name] {
			return false
		}
		return true
	case *ast.CallExpr:
		return true // seq.Task("x").Define(...)
	case *ast.SelectorExpr:
		return isLikelyEvoTaskReceiver(v.X, disproven)
	case *ast.ParenExpr:
		return isLikelyEvoTaskReceiver(v.X, disproven)
	default:
		return true
	}
}

// evoLocalStructNames collects every struct type this file declares — Evo
// never hands back a locally-declared struct, so a variable proven to hold
// one of these is proven not to be an Evo handle (spec §57).
func evoLocalStructNames(file *ast.File) map[string]bool {
	names := map[string]bool{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if _, isStruct := ts.Type.(*ast.StructType); isStruct {
				names[ts.Name.Name] = true
			}
		}
	}
	return names
}

// evoDisprovenVars collects every identifier this file assigns from a
// composite literal (bare or address-of) of one of its own locally-declared
// struct types — structural proof the identifier cannot hold a real Evo
// handle no matter what it is named, replacing a name-based guess with a
// fact the AST already states (spec §57).
func evoDisprovenVars(file *ast.File) map[string]bool {
	localStructs := evoLocalStructNames(file)
	disproven := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, rhs := range assign.Rhs {
			if i >= len(assign.Lhs) {
				continue
			}
			lhs, ok := assign.Lhs[i].(*ast.Ident)
			if !ok || !isLocalStructLiteral(rhs, localStructs) {
				continue
			}
			disproven[lhs.Name] = true
		}
		return true
	})
	return disproven
}

// isLocalStructLiteral reports whether e is `LocalType{...}` or
// `&LocalType{...}` for a type name present in localStructs.
func isLocalStructLiteral(e ast.Expr, localStructs map[string]bool) bool {
	if u, ok := e.(*ast.UnaryExpr); ok && u.Op == token.AND {
		e = u.X
	}
	cl, ok := e.(*ast.CompositeLit)
	if !ok {
		return false
	}
	id, ok := cl.Type.(*ast.Ident)
	return ok && localStructs[id.Name]
}

// ===== EVO-EVIDENCE-001: a legacy named task.Evidence("name", func() error
// { ... }) callback performs a raw mutation. Evidence is superseded as a
// boolean current-state conclusion; it was never the place mutation happens
// (spec §2, §56, §57).

func detectMutatingLegacyEvidence(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	disproven := evoDisprovenVars(file)
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, fl, ok := funcLitArgAt(call, "Evidence", 2, 1, disproven)
		if !ok {
			return true
		}
		pos, name, mutates := firstRawMutationCall(fl.Body)
		if !mutates {
			return true
		}
		recv := exprDottedNameOrDefault(sel.X, "task")
		findings = append(findings, Finding{
			RuleID:          "EVO-EVIDENCE-001",
			Severity:        "error",
			Message:         recv + ".Evidence's callback calls " + name + "; the legacy named-Evidence shape is superseded and was never the place mutation belongs",
			File:            filename,
			Line:            fset.Position(pos).Line,
			Column:          fset.Position(pos).Column,
			Suggestion:      "move " + name + " into " + recv + ".Define(func(ctx context.Context) error { ... }); add " + recv + ".Verify(func(ctx context.Context) (bool, error) { ... }) only if the resulting state can be observed directly",
			RequiredVersion: evoDagEvidenceRequiredVersion,
		})
		return true
	})
	return findings
}

// ===== EVO-VERIFY-001: a task.Verify(func(ctx) (bool, error) { ... })
// callback performs a raw mutation. Verify must be read-only — a verifier
// error prevents blind mutation, but a mutating verifier can never be
// safely retried or ANDed with another verifier (spec §9.1, §57).

func detectMutatingVerify(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	disproven := evoDisprovenVars(file)
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, fl, ok := funcLitArgAt(call, "Verify", 1, 0, disproven)
		if !ok {
			return true
		}
		pos, name, mutates := firstRawMutationCall(fl.Body)
		if !mutates {
			return true
		}
		recv := exprDottedNameOrDefault(sel.X, "task")
		findings = append(findings, Finding{
			RuleID:          "EVO-VERIFY-001",
			Severity:        "error",
			Message:         recv + ".Verify's callback calls " + name + "; Verify must be read-only",
			File:            filename,
			Line:            fset.Position(pos).Line,
			Column:          fset.Position(pos).Column,
			Suggestion:      "move " + name + " into " + recv + ".Define(...) and keep " + recv + ".Verify(...) limited to observation, e.g. `return checkFn(ctx)`",
			RequiredVersion: evoDagEvidenceRequiredVersion,
		})
		return true
	})
	return findings
}

// ===== EVO-DRYRUN-001: a task.Define(func(ctx) error { ... }) callback
// performs a raw mutation. Evo cannot intercept an arbitrary Go side effect
// (os.WriteFile, a raw exec.Command, a direct database mutation); code that
// promises Evo dry-run safety must route mutation through evo.File,
// evo.Exec, or a typed mutation boundary instead (spec §32.2, §57).

func detectRawMutationInDefine(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	disproven := evoDisprovenVars(file)
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, fl, ok := funcLitArgAt(call, "Define", 1, 0, disproven)
		if !ok {
			return true
		}
		pos, name, mutates := firstRawMutationCall(fl.Body)
		if !mutates {
			return true
		}
		recv := exprDottedNameOrDefault(sel.X, "task")
		findings = append(findings, Finding{
			RuleID:          "EVO-DRYRUN-001",
			Severity:        "error",
			Message:         recv + ".Define raw-calls " + name + "; Evo cannot intercept an arbitrary side effect, so this callback is unsafe under dry-run",
			File:            filename,
			Line:            fset.Position(pos).Line,
			Column:          fset.Position(pos).Column,
			Suggestion:      "route the mutation through evo.File(ctx, evo.FileSpec{...}), evo.Exec(ctx, evo.ExecSpec{...}), or a typed mutation verb instead of calling " + name + " directly",
			RequiredVersion: evoDagEvidenceRequiredVersion,
		})
		return true
	})
	return findings
}

// exprDottedNameOrDefault is exprDottedName with a fallback for the rare
// receiver shape exprDottedName cannot spell (e.g. a call result), so
// Suggestion text always names a concrete-looking receiver.
func exprDottedNameOrDefault(e ast.Expr, fallback string) string {
	if name := exprDottedName(e); name != "" {
		return name
	}
	return fallback
}

// stringLit unquotes e when it is a string literal.
func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}
