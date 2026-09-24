// Package review — API-062 (1.2/ZYS-945): a caller stopwatch fed into a
// Task's Summary or a Fact. Evo stamps every Task's lifecycle boundaries
// and derives the run aggregate (TaskSnapshot.Timing, Conclusion.Metrics,
// JSON timing and data.metrics, the Verbose timing line), so hand-rolled
// duration prose duplicates that truth as a string no machine consumer can
// read.
//
// A stopwatch is a time.Now() the same function stored in a local, later
// read with time.Since(start) or time.Now().Sub(start) — directly in the
// Summary/Fact argument, or through one local that holds the reading
// (elapsed := time.Since(start)). A duration of a domain timestamp (a
// file's age, time.Since(info.ModTime())) is a fact about the domain, not
// a stopwatch, and stays silent; so does a stopwatch that drives domain
// logic (a deadline) and never reaches Summary/Fact.
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
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		sw := newStopwatches(fn.Body)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !timingNarrationMethods[sel.Sel.Name] || !isLikelyEvoReceiver(sel.X) {
				return true
			}
			if sw.anyReads(call.Args) {
				findings = append(findings, manualTaskTimingFinding(filename, fset.Position(call.Pos()), sel))
			}
			return true
		})
	}
	return findings
}

// stopwatches are one function's stopwatch locals: starts hold a
// time.Now(), readings hold a stopwatch read of a start.
type stopwatches struct {
	starts   map[string]bool
	readings map[string]bool
}

// newStopwatches collects body's starts, then the locals assigned a
// reading of one (the one hop an elapsed variable adds).
func newStopwatches(body *ast.BlockStmt) stopwatches {
	sw := stopwatches{starts: map[string]bool{}, readings: map[string]bool{}}
	forEachLocalAssignment(body, func(name string, value ast.Expr) {
		if call, ok := value.(*ast.CallExpr); ok && isTimeNow(call) {
			sw.starts[name] = true
		}
	})
	forEachLocalAssignment(body, func(name string, value ast.Expr) {
		if sw.readsStart(value) {
			sw.readings[name] = true
		}
	})
	return sw
}

// forEachLocalAssignment visits every name = value pairing in body, from
// assignments and var declarations alike.
func forEachLocalAssignment(body *ast.BlockStmt, visit func(name string, value ast.Expr)) {
	pair := func(names []ast.Expr, values []ast.Expr) {
		if len(names) != len(values) {
			return
		}
		for i, lhs := range names {
			if id, ok := lhs.(*ast.Ident); ok {
				visit(id.Name, values[i])
			}
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			pair(s.Lhs, s.Rhs)
		case *ast.ValueSpec:
			names := make([]ast.Expr, len(s.Names))
			for i, id := range s.Names {
				names[i] = id
			}
			pair(names, s.Values)
		}
		return true
	})
}

// anyReads reports whether any argument, at any depth, reads a stopwatch
// directly or names a local holding a reading.
func (sw stopwatches) anyReads(args []ast.Expr) bool {
	for _, arg := range args {
		if sw.readsStart(arg) || sw.namesReading(arg) {
			return true
		}
	}
	return false
}

func (sw stopwatches) namesReading(expr ast.Expr) bool {
	return containsNode(expr, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		return ok && sw.readings[id.Name]
	})
}

// readsStart reports whether expr contains time.Since(start) or
// time.Now().Sub(start) for one of this function's starts.
func (sw stopwatches) readsStart(expr ast.Expr) bool {
	return containsNode(expr, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 || !isStopwatchRead(call) {
			return false
		}
		start, ok := call.Args[0].(*ast.Ident)
		return ok && sw.starts[start.Name]
	})
}

func containsNode(root ast.Node, match func(ast.Node) bool) bool {
	found := false
	ast.Inspect(root, func(n ast.Node) bool {
		if found || n == nil {
			return false
		}
		found = match(n)
		return !found
	})
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
	return ok && sel.Sel.Name == "Sub" && isTimeNow(inner)
}

func isTimeNow(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && len(call.Args) == 0 && isPkgSel(sel, "time", "Now")
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
