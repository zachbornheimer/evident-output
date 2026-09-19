// Package review — invented scheduling around Evo: a custom lock table
// (EVO-DAG-004), a goroutine whose job is Task.Wait (EVO-DAG-005),
// resource contention encoded as After (EVO-DAG-006), and an app
// heartbeat ticker that pokes Doing/Progress (EVO-LIVE-002).
package review

import (
	"go/ast"
	"go/token"
	"strings"
)

const (
	fixScheduleMarker = "fix-schedule"
	writeLocksIdent   = "writeLocks"
	readLocksIdent    = "readLocks"
)

var waitReceiverDeny = map[string]bool{
	"wg": true, "waitGroup": true, "waitgroup": true, "g": true, "eg": true,
	"errgroup": true, "errGroup": true,
}

func detectCustomSchedulerAroundEvo(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.Ident:
			if v.Name != writeLocksIdent && v.Name != readLocksIdent {
				return true
			}
			pos := fset.Position(v.Pos())
			findings = append(findings, stampFinding(
				"EVO-DAG-004", "warning",
				"a custom "+v.Name+" table around Evo reimplements process-local resource holds",
				"delete the "+v.Name+" scheduler; evo.File/evo.Patch already serialize same-path work through process-local resource holds",
				filename, pos,
			))
		case *ast.BasicLit:
			s, ok := stringLit(v)
			if !ok || !strings.Contains(s, fixScheduleMarker) {
				return true
			}
			pos := fset.Position(v.Pos())
			findings = append(findings, stampFinding(
				"EVO-DAG-004", "warning",
				"a synthetic //fix-schedule/ path around Evo reimplements the scheduler",
				`delete the "//fix-schedule/" path; declare work with Group.Task(name).Define and evo.File — Evo already schedules and serializes same-path File/Patch`,
				filename, pos,
			))
		}
		return true
	})
	return findings
}

func detectGoroutineWaitingOnTaskWait(filename string, file *ast.File, fset *token.FileSet) []Finding {
	disproven := evoDisprovenVars(file)
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		goStmt, ok := n.(*ast.GoStmt)
		if !ok {
			return true
		}
		pos := fset.Position(goStmt.Pos())
		if isTaskWaitCall(goStmt.Call, disproven) {
			recv := waitReceiverName(goStmt.Call)
			findings = append(findings, stampFinding(
				"EVO-DAG-005", "warning",
				"a caller-owned goroutine waits on Task.Wait solely to sequence Evo work",
				"delete the goroutine and declare the work under evo.Sequence(\"apply\") so ordering is a scheduler edge, not "+recv+".Wait()",
				filename, pos,
			))
			return true
		}
		fl, ok := goStmt.Call.Fun.(*ast.FuncLit)
		if !ok || fl.Body == nil {
			return true
		}
		if goBodyDefines(fl.Body) || !goBodyWaitsOnTask(fl.Body, disproven) {
			return true
		}
		recv := firstWaitReceiver(fl.Body, disproven)
		findings = append(findings, stampFinding(
			"EVO-DAG-005", "warning",
			"a caller-owned goroutine waits on Task.Wait solely to sequence Evo work",
			"delete the goroutine and declare the work under evo.Sequence(\"apply\") so ordering is a scheduler edge, not "+recv+".Wait()",
			filename, pos,
		))
		return true
	})
	return findings
}

func isTaskWaitCall(call *ast.CallExpr, disproven map[string]bool) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Wait" {
		return false
	}
	recv := exprDottedName(sel.X)
	if waitReceiverDeny[recv] {
		return false
	}
	return isLikelyEvoTaskReceiver(sel.X, disproven)
}

func waitReceiverName(call *ast.CallExpr) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "task"
	}
	return exprDottedNameOrDefault(sel.X, "task")
}

func goBodyDefines(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if ok && sel.Sel.Name == "Define" {
			found = true
		}
		return !found
	})
	return found
}

func goBodyWaitsOnTask(body *ast.BlockStmt, disproven map[string]bool) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if isTaskWaitCall(call, disproven) {
			found = true
		}
		return !found
	})
	return found
}

func firstWaitReceiver(body *ast.BlockStmt, disproven map[string]bool) string {
	recv := "task"
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if isTaskWaitCall(call, disproven) {
			recv = waitReceiverName(call)
			return false
		}
		return true
	})
	return recv
}

func detectResourceContentionEncodedAsAfter(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	disproven := evoDisprovenVars(file)
	var findings []Finding
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		findings = append(findings, dag006InBody(filename, fd.Body, fset, disproven, pkg)...)
	}
	return findings
}

func dag006InBody(filename string, body *ast.BlockStmt, fset *token.FileSet, disproven map[string]bool, pkg string) []Finding {
	paths := map[string]map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Define" || len(call.Args) == 0 {
			return true
		}
		if !isLikelyEvoTaskReceiver(sel.X, disproven) {
			return true
		}
		recv := exprDottedName(sel.X)
		if recv == "" {
			return true
		}
		fl, ok := call.Args[0].(*ast.FuncLit)
		if !ok || fl.Body == nil {
			return true
		}
		for _, key := range filePathKeysIn(fl.Body, pkg) {
			if paths[recv] == nil {
				paths[recv] = map[string]bool{}
			}
			paths[recv][key] = true
		}
		return true
	})
	var findings []Finding
	for _, edge := range collectAfterEdgesFromBody(body, disproven) {
		childPaths := paths[edge.child]
		parentPaths := paths[edge.parent]
		if !sharePathKey(childPaths, parentPaths) {
			continue
		}
		pos := fset.Position(edge.pos)
		findings = append(findings, stampFinding(
			"EVO-DAG-006", "warning",
			"After encodes resource contention; evo.File already serializes same-path work",
			"delete "+edge.child+".After("+edge.parent+") — evo.File already serializes same-path work through process-local resource holds; After is a DAG edge, not a lock",
			filename, pos,
		))
	}
	return findings
}

func collectAfterEdgesFromBody(body *ast.BlockStmt, disproven map[string]bool) []afterEdge {
	var edges []afterEdge
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "After" || len(call.Args) != 1 || !isLikelyEvoTaskReceiver(sel.X, disproven) {
			return true
		}
		child := exprDottedName(sel.X)
		parent, ok := call.Args[0].(*ast.Ident)
		if !ok || child == "" {
			return true
		}
		edges = append(edges, afterEdge{child: child, parent: parent.Name, pos: call.Pos()})
		return true
	})
	return edges
}

func sharePathKey(a, b map[string]bool) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	for k := range a {
		if b[k] {
			return true
		}
	}
	return false
}

func filePathKeysIn(body *ast.BlockStmt, pkg string) []string {
	var keys []string
	ast.Inspect(body, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || !isFileSpecLit(lit, pkg) {
			return true
		}
		fields := compositeKV(lit)
		path, ok := fields["Path"]
		if !ok {
			return true
		}
		if key := exprKey(path); key != "" {
			keys = append(keys, key)
		}
		return true
	})
	return keys
}

func detectAppHeartbeatTicker(filename string, file *ast.File, fset *token.FileSet) []Finding {
	disproven := evoDisprovenVars(file)
	var findings []Finding
	forEachFuncBody(file, func(body *ast.BlockStmt) {
		if !callsTimeNewTicker(body) {
			return
		}
		ast.Inspect(body, func(n ast.Node) bool {
			var loopBody *ast.BlockStmt
			switch s := n.(type) {
			case *ast.ForStmt:
				loopBody = s.Body
			case *ast.RangeStmt:
				loopBody = s.Body
			}
			if loopBody == nil {
				return true
			}
			if !loopCallsDoingOrProgress(loopBody, disproven) {
				return true
			}
			pos := fset.Position(n.Pos())
			findings = append(findings, stampFinding(
				"EVO-LIVE-002", "warning",
				"time.NewTicker is used to call Doing/Progress just to keep the UI alive; Evo already heartbeats the live region",
				`delete the time.NewTicker loop; Evo heartbeats the live region itself — call task.Doing("pushing feat/a") only when the activity text actually changes`,
				filename, pos,
			))
			return true
		})
	})
	return findings
}

func callsTimeNewTicker(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if calledFuncDotted(call) == "time.NewTicker" {
			found = true
		}
		return !found
	})
	return found
}

func loopCallsDoingOrProgress(body *ast.BlockStmt, disproven map[string]bool) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name != "Doing" && sel.Sel.Name != "Progress" {
			return true
		}
		if isLikelyEvoTaskReceiver(sel.X, disproven) {
			found = true
		}
		return !found
	})
	return found
}

func isFileSpecLit(lit *ast.CompositeLit, pkg string) bool {
	switch t := lit.Type.(type) {
	case *ast.Ident:
		return t.Name == "FileSpec"
	case *ast.SelectorExpr:
		return t.Sel.Name == "FileSpec" && (pkg == "" || exprDottedName(t.X) == pkg)
	default:
		return false
	}
}

func compositeKV(lit *ast.CompositeLit) map[string]ast.Expr {
	out := map[string]ast.Expr{}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		id, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		out[id.Name] = kv.Value
	}
	return out
}
