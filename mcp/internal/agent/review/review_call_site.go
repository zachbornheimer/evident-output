package review

import (
	"go/ast"
	"go/token"
)

const ruleInlineConstruct = "CALL-001"

// evoRunEntrypoints are the package-level calls that take a Config.
var evoRunEntrypoints = []string{"Main", "Execute"}

// detectInlineConstructAtEvoCall flags make() or new() constructed inside an
// evo.Main or evo.Execute argument. A value built inline cannot be read back
// after the run (a buffer behind Facades.Stdout, a LevelVar behind
// Config.Debug); a named local bound before the call is the clean form.
func detectInlineConstructAtEvoCall(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" {
		return nil
	}
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isEvoEntrypointCall(call, pkg) {
			return true
		}
		builtin, pos := firstMakeOrNewInArgs(call.Args)
		if builtin == "" {
			return true
		}
		findings = append(findings, Finding{
			RuleID:     ruleInlineConstruct,
			Message:    "inline " + builtin + "() inside an " + pkg + ".Main/" + pkg + ".Execute argument; extract a named local before the call",
			File:       filename,
			Line:       fset.Position(pos).Line,
			Suggestion: "extract " + builtin + "(...) to a named local, then pass that local in the Config",
		})
		return true
	})
	return findings
}

func isEvoEntrypointCall(call *ast.CallExpr, pkg string) bool {
	for _, name := range evoRunEntrypoints {
		if isEvoPackageCall(call, pkg, name) {
			return true
		}
	}
	return false
}

func firstMakeOrNewInArgs(args []ast.Expr) (string, token.Pos) {
	for _, arg := range args {
		var found string
		var pos token.Pos
		ast.Inspect(arg, func(n ast.Node) bool {
			if found != "" {
				return false
			}
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			id, ok := c.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			if id.Name == "make" || id.Name == "new" {
				found = id.Name
				pos = id.Pos()
				return false
			}
			return true
		})
		if found != "" {
			return found, pos
		}
	}
	return "", token.NoPos
}
