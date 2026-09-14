package review

import (
	"go/ast"
	"go/token"
)

const ruleInlineConstruct = "CALL-001"

var evoConstructMethods = map[string]bool{
	"Init":  true,
	"Task":  true,
	"Group": true,
}

// detectInlineConstructAtEvoCall flags make() or new() constructed inside
// an evo.Init/Task/Group argument. A named local bound before the call is
// the clean form — the builtin is then outside the call's argument tree.
func detectInlineConstructAtEvoCall(filename string, fset *token.FileSet, file *ast.File) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !isEvoConstructCall(call) {
			return true
		}
		builtin, pos := firstMakeOrNewInArgs(call.Args)
		if builtin == "" {
			return true
		}
		findings = append(findings, Finding{
			RuleID:     ruleInlineConstruct,
			Severity:   "warning",
			Message:    "inline " + builtin + "() inside evo.Init/Task/Group argument; extract a named local before the call",
			File:       filename,
			Line:       fset.Position(pos).Line,
			Suggestion: "extract " + builtin + "(...) to a named local, then pass that local into the evo.Init/Task/Group call",
		})
		return true
	})
	return findings
}

func isEvoConstructCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if !evoConstructMethods[sel.Sel.Name] {
		return false
	}
	return isLikelyEvoReceiver(sel.X)
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
