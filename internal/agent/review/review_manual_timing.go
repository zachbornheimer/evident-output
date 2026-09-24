// Package review — API-062 (1.2/ZYS-945): a caller stopwatch — time.Since
// or a time.Now().Sub — fed into a Task's Summary or a Fact. Evo stamps
// every Task's lifecycle boundaries and derives the run aggregate
// (TaskSnapshot.Timing, Conclusion.Metrics, JSON timing and data.metrics,
// the Verbose timing line), so hand-rolled duration prose duplicates that
// truth as a string no machine consumer can read. A stopwatch that drives
// domain logic (a deadline) and never reaches Summary/Fact stays silent.
package review

import (
	"go/ast"
	"go/token"
	"go/types"
)

// timingNarrationMethods are the metadata verbs a stopwatch reading gets
// narrated through.
var timingNarrationMethods = map[string]bool{"Summary": true, "Fact": true}

// detectManualTaskTiming is API-062.
func detectManualTaskTiming(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !timingNarrationMethods[sel.Sel.Name] || !isLikelyEvoReceiver(sel.X) {
			return true
		}
		if !anyArgReadsStopwatch(call.Args) {
			return true
		}
		findings = append(findings, manualTaskTimingFinding(filename, fset.Position(call.Pos()), sel))
		return true
	})
	return findings
}

// anyArgReadsStopwatch reports whether any argument, at any depth, calls
// time.Since or subtracts from time.Now().
func anyArgReadsStopwatch(args []ast.Expr) bool {
	found := false
	for _, arg := range args {
		ast.Inspect(arg, func(n ast.Node) bool {
			if found {
				return false
			}
			if call, ok := n.(*ast.CallExpr); ok && isStopwatchRead(call) {
				found = true
			}
			return !found
		})
	}
	return found
}

// isStopwatchRead is time.Since(x) or time.Now().Sub(x).
func isStopwatchRead(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if isPkgSel(sel, "time", "Since") {
		return true
	}
	inner, ok := sel.X.(*ast.CallExpr)
	if !ok || sel.Sel.Name != "Sub" {
		return false
	}
	now, ok := inner.Fun.(*ast.SelectorExpr)
	return ok && isPkgSel(now, "time", "Now")
}

func isPkgSel(sel *ast.SelectorExpr, pkg, name string) bool {
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == pkg && sel.Sel.Name == name
}

func manualTaskTimingFinding(filename string, pos token.Position, sel *ast.SelectorExpr) Finding {
	recv := types.ExprString(sel.X)
	return Finding{
		RuleID:   "API-062",
		Severity: "warning",
		Message:  recv + "." + sel.Sel.Name + " narrates a caller stopwatch; Evo already stamps every Task's lifecycle and derives where the run's time went",
		File:     filename,
		Line:     pos.Line,
		Column:   pos.Column,
		Suggestion: "delete the stopwatch and this " + sel.Sel.Name + " call; read TaskSnapshot.Timing (Queued/Running/Total) or out.Conclusion().Metrics() in code, " +
			"\"timing\"/\"data.metrics\" in JSON, and the Verbose timing line in human output",
		RequiredVersion: dialectOneTwo,
	}
}
