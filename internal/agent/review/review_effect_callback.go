// Package review — API-042 and API-043: content-free or no-op evo.Effect callbacks and plural Effect objects.
package review

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// ===== API-042: an evo.Effect callback is nil, or a no-op — the work
// already happened elsewhere and the callback is theater over it (zq
// README.md:39's nil callback, setup_python.go:172-181's callback that only
// returns installedPythonModuleCount(...)).

// effectCallbackArg is evo.Effect's callback argument index:
// Effect(ctx, spec, fn).
const effectCallbackArg = 2

func detectNoOpEffectCallback(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	funcs := map[string]*ast.FuncDecl{}
	ast.Inspect(file, func(n ast.Node) bool {
		if fd, ok := n.(*ast.FuncDecl); ok && fd.Body != nil {
			funcs[fd.Name.Name] = fd
		}
		return true
	})

	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isEvoEffectCall(call, pkg) {
			return true
		}
		pos := fset.Position(call.Pos())
		if shape, ok := noOpCallbackShape(call.Args[effectCallbackArg], funcs); ok {
			findings = append(findings, noOpEffectFinding(filename, pos, shape))
		}
		return true
	})
	return findings
}

// isEvoEffectCall reports whether call is pkg.Effect(ctx, spec, fn).
func isEvoEffectCall(call *ast.CallExpr, pkg string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && pkg != "" && isEvoIdent(sel.X, pkg) && sel.Sel.Name == "Effect" && len(call.Args) > effectCallbackArg
}

// noOpCallbackShape classifies fn as a callback that does no real work:
// "nil", a bare `return nil` "literal", or a "named" func (directly or as a
// literal's single delegated return) whose body only validates.
func noOpCallbackShape(fn ast.Expr, funcs map[string]*ast.FuncDecl) (string, bool) {
	switch arg := fn.(type) {
	case *ast.Ident:
		if arg.Name == "nil" {
			return "nil", true
		}
		if fd, ok := funcs[arg.Name]; ok && funcBodyLooksLikeNoOpWork(fd.Body) {
			return "named", true
		}
	case *ast.FuncLit:
		if funcBodyIsBareReturnNil(arg.Body) {
			return "literal", true
		}
		if name, ok := singleReturnCallName(arg.Body); ok {
			if fd, ok := funcs[name]; ok && funcBodyLooksLikeNoOpWork(fd.Body) {
				return "named", true
			}
		}
	}
	return "", false
}

// noOpCalleeAllowList are calls cheap enough to still count as "no real
// work" — validation and error construction, not I/O or mutation.
var noOpCalleeAllowList = map[string]bool{
	"fmt.Errorf": true, "errors.New": true, "len": true,
}

// funcBodyLooksLikeNoOpWork reports whether body's only calls are
// validation/error-construction (the noOpCalleeAllowList) — i.e. it never
// calls out to do the mutation's actual work (zq's installedPythonModuleCount
// only checks a count that was already computed by the caller).
func funcBodyLooksLikeNoOpWork(body *ast.BlockStmt) bool {
	hasRealCall := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !noOpCalleeAllowList[calledFuncDotted(call)] {
			hasRealCall = true
		}
		return true
	})
	return !hasRealCall
}

// singleReturnCallName reports the called function's bare/method name when
// body is exactly one statement, `return someFunc(...)`.
func singleReturnCallName(body *ast.BlockStmt) (string, bool) {
	if len(body.List) != 1 {
		return "", false
	}
	ret, ok := body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return "", false
	}
	call, ok := ret.Results[0].(*ast.CallExpr)
	if !ok {
		return "", false
	}
	name := calledFuncName(call)
	return name, name != ""
}

func funcBodyIsBareReturnNil(body *ast.BlockStmt) bool {
	if len(body.List) != 1 {
		return false
	}
	ret, ok := body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	id, ok := ret.Results[0].(*ast.Ident)
	return ok && id.Name == "nil"
}

func noOpEffectFinding(filename string, pos token.Position, shape string) Finding {
	message := "evo.Effect has a nil callback; the mutation must run inside the callback"
	if shape != "nil" {
		message = "evo.Effect's callback does no real work (only validates/constructs an error); the mutation already ran elsewhere"
	}
	return Finding{
		RuleID:     "API-042",
		Message:    message,
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: "move the mutation itself inside the Effect callback (or an Evo-native File/Patch); do not report it after the fact with Record/RecordLabel/RecordName — those have no record-only replacement (ZYS-974)",
	}
}

// ===== API-043: a plural EffectSpec.Object literal — evo pluralizes the
// singular from Quantity; passing the plural already produces "deleted 1
// worktrees" (zq axis-14 P17) because an already-plural literal round-trips
// unchanged.

// mutationObjectNouns is the small, deliberately narrow whitelist of Effect object
// nouns this detector recognizes — it only fires when trimming a candidate
// plural suffix yields one of these, so it never guesses at English
// pluralization rules for words it doesn't know (see isSibilantPlural).
var mutationObjectNouns = map[string]bool{
	"worktree": true, "branch": true, "module": true, "package": true,
	"file": true, "directory": true, "dir": true, "tag": true,
	"remote": true, "repo": true, "repository": true, "config": true,
	"lock": true, "cache": true, "session": true, "log": true,
	"artifact": true, "dependency": true, "venv": true, "executable": true,
	"credential": true, "secret": true, "token": true, "key": true,
	"hook": true, "plugin": true, "template": true, "workspace": true,
	"container": true, "job": true, "entry": true, "record": true,
	"snapshot": true, "backup": true, "release": true, "build": true,
	"target": true,
}

func detectPluralEffectObject(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok || !isEvoEffectSpecLit(cl, pkg) {
			return true
		}
		lit, ok := effectSpecObjectLit(cl)
		if !ok {
			return true
		}
		text, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		singular, ok := pluralObjectSingular(text)
		if !ok {
			return true
		}
		pos := fset.Position(lit.Pos())
		findings = append(findings, Finding{
			RuleID:     "API-043",
			Message:    "EffectSpec.Object literal " + strconv.Quote(text) + " is plural; evo pluralizes the singular from Quantity",
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Suggestion: "replace Object: " + strconv.Quote(text) + " with Object: " + strconv.Quote(singular),
		})
		return true
	})
	return findings
}

// isEvoEffectSpecLit reports whether cl is a pkg.EffectSpec{...} literal.
func isEvoEffectSpecLit(cl *ast.CompositeLit, pkg string) bool {
	sel, ok := cl.Type.(*ast.SelectorExpr)
	return ok && pkg != "" && isEvoIdent(sel.X, pkg) && sel.Sel.Name == "EffectSpec"
}

// effectSpecObjectLit returns the string literal keyed Object in cl.
func effectSpecObjectLit(cl *ast.CompositeLit) (*ast.BasicLit, bool) {
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok || identName(kv.Key) != "Object" {
			continue
		}
		lit, ok := kv.Value.(*ast.BasicLit)
		return lit, ok && lit.Kind == token.STRING
	}
	return nil, false
}

// pluralObjectSingular returns the singular form when word is a recognized
// plural object noun (mutationObjectNouns), else ("", false).
func pluralObjectSingular(word string) (string, bool) {
	lower := strings.ToLower(word)
	candidates := []string{}
	if strings.HasSuffix(lower, "ies") && len(word) > 3 {
		candidates = append(candidates, word[:len(word)-3]+"y")
	}
	if strings.HasSuffix(lower, "es") && len(word) > 2 {
		candidates = append(candidates, word[:len(word)-2])
	}
	if strings.HasSuffix(lower, "s") && len(word) > 1 {
		candidates = append(candidates, word[:len(word)-1])
	}
	for _, c := range candidates {
		if mutationObjectNouns[strings.ToLower(c)] {
			return c, true
		}
	}
	return "", false
}
