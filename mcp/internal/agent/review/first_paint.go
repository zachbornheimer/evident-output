package review

import (
	"go/ast"
	"go/token"
	"strings"
)

// heavyIOCalls are calls heavy enough to blank the terminal for a visible
// interval when they run ahead of the first paint.
var heavyIOCalls = map[string]bool{
	"os.ReadFile": true, "os.ReadDir": true, "os.Open": true, "filepath.Walk": true,
	"filepath.WalkDir": true, "exec.Command": true, "http.Get": true, "net.Dial": true,
}

// heavyIOMethods are domain methods that do heavy I/O. Domain inventory
// (purge.Inventory) is the canary that stdlib-only names missed.
var heavyIOMethods = map[string]bool{"Inventory": true}

// loopWalkPrefixes are domain walk helpers that count as loop work even
// without a stdlib I/O spelling.
var loopWalkPrefixes = []string{"FindWorktree", "FindCache", "ListWorktree", "WalkDir", "Walk"}

// entityMethods declare a presentation entity on the run or a container.
var entityMethods = map[string]bool{"Task": true, "Group": true, "Sequence": true}

// heavyIOName names the heavy I/O call n makes, or "".
func heavyIOName(n ast.Node) string {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return ""
	}
	name := exprDottedName(call.Fun)
	if heavyIOCalls[name] {
		return name
	}
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok && heavyIOMethods[sel.Sel.Name] {
		return "." + sel.Sel.Name
	}
	return ""
}

// loopWorkName names the I/O or walk call n makes, or "".
func loopWorkName(n ast.Node) string {
	if name := heavyIOName(n); name != "" {
		return name
	}
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return ""
	}
	callee := exprDottedName(call.Fun)
	if i := strings.LastIndex(callee, "."); i >= 0 {
		callee = callee[i+1:]
	}
	for _, prefix := range loopWalkPrefixes {
		if strings.HasPrefix(callee, prefix) {
			return callee
		}
	}
	return ""
}

// inspectOutsideClosures visits root's nodes except those inside a function
// literal, which does not run where it is written.
func inspectOutsideClosures(root ast.Node, visit func(ast.Node)) {
	ast.Inspect(root, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok && n != root {
			return false
		}
		visit(n)
		return true
	})
}

// forEachFunc calls fn for every function declaration and literal body.
func forEachFunc(file *ast.File, fn func(*ast.FuncType, *ast.BlockStmt)) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch f := n.(type) {
		case *ast.FuncDecl:
			if f.Body != nil {
				fn(f.Type, f.Body)
			}
		case *ast.FuncLit:
			fn(f.Type, f.Body)
		}
		return true
	})
}

// runParamName is the name of the *evo.Run parameter of ft, or "".
func runParamName(ft *ast.FuncType, pkg string) string {
	for _, field := range ft.Params.List {
		star, ok := field.Type.(*ast.StarExpr)
		if !ok || len(field.Names) == 0 {
			continue
		}
		if sel, ok := star.X.(*ast.SelectorExpr); ok && sel.Sel.Name == "Run" && isEvoIdent(sel.X, pkg) {
			return field.Names[0].Name
		}
	}
	return ""
}

// firstEntityPos is where body first declares a Task, Group or Sequence on
// the run named run, outside any closure, or token.NoPos.
func firstEntityPos(body *ast.BlockStmt, run string) token.Pos {
	first := token.NoPos
	inspectOutsideClosures(body, func(n ast.Node) {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if ok && entityMethods[sel.Sel.Name] && rootIdentName(sel.X) == run && (!first.IsValid() || call.Pos() < first) {
			first = call.Pos()
		}
	})
	return first
}

// rootIdentName is the leftmost identifier of a selector or call chain.
func rootIdentName(e ast.Expr) string {
	for {
		switch v := e.(type) {
		case *ast.Ident:
			return v.Name
		case *ast.SelectorExpr:
			e = v.X
		case *ast.CallExpr:
			e = v.Fun
		default:
			return ""
		}
	}
}

// detectFirstPaintGaps flags heavy I/O that runs before the run is armed or
// before the run body declares its first entity. FP-001: in a function that
// calls evo.Main or evo.Execute, I/O ahead of that call blanks the terminal
// because nothing paints yet. FP-002: in a run body (a function taking
// *evo.Run), I/O ahead of the first Task, Group or Sequence leaves the
// armed run showing nothing.
func detectFirstPaintGaps(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" {
		return nil
	}
	var findings []Finding
	forEachFunc(file, func(ft *ast.FuncType, body *ast.BlockStmt) {
		if start := runEntrypointPos(body, pkg); start.IsValid() {
			if io, name := firstHeavyIOBefore(body, start); io.IsValid() {
				findings = append(findings, firstPaintFinding(filename, fset, "FP-001", io, name,
					"heavy I/O runs before "+pkg+".Main/"+pkg+".Execute is called; nothing is armed to paint within 100ms of process start",
					"call "+pkg+".Main first and move "+name+"(...) into the first Task's Define"))
			}
		}
		run := runParamName(ft, pkg)
		if run == "" {
			return
		}
		if entity := firstEntityPos(body, run); entity.IsValid() {
			if io, name := firstHeavyIOBefore(body, entity); io.IsValid() {
				findings = append(findings, firstPaintFinding(filename, fset, "FP-002", io, name,
					"heavy I/O runs in the run body before its first Task/Group/Sequence; declare the first entity before this I/O",
					"declare the first Task/Group/Sequence before "+name+"(...)"))
			}
		}
	})
	return findings
}

// runEntrypointPos is where body calls evo.Main or evo.Execute outside any
// closure, or token.NoPos.
func runEntrypointPos(body *ast.BlockStmt, pkg string) token.Pos {
	pos := token.NoPos
	inspectOutsideClosures(body, func(n ast.Node) {
		if call, ok := n.(*ast.CallExpr); ok && isEvoEntrypointCall(call, pkg) && !pos.IsValid() {
			pos = call.Pos()
		}
	})
	return pos
}

// firstHeavyIOBefore is the earliest heavy I/O call in body, outside
// closures, positioned before limit.
func firstHeavyIOBefore(body *ast.BlockStmt, limit token.Pos) (token.Pos, string) {
	first, name := token.NoPos, ""
	inspectOutsideClosures(body, func(n ast.Node) {
		if io := heavyIOName(n); io != "" && n.Pos() < limit && (!first.IsValid() || n.Pos() < first) {
			first, name = n.Pos(), io
		}
	})
	return first, name
}

func firstPaintFinding(filename string, fset *token.FileSet, ruleID string, at token.Pos, name, message, suggestion string) Finding {
	pos := fset.Position(at)
	return Finding{RuleID: ruleID, Message: message, File: filename, Line: pos.Line, Column: pos.Column, Suggestion: suggestion}
}

// detectSilentPreTaskLoops flags a range loop that does I/O or walks disk
// in a run body before its first Task, Group or Sequence. That is the
// purge/prune silent class: scanning looks dead because the loop never
// lived inside a task definition.
func detectSilentPreTaskLoops(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" {
		return nil
	}
	var findings []Finding
	forEachFunc(file, func(ft *ast.FuncType, body *ast.BlockStmt) {
		run := runParamName(ft, pkg)
		if run == "" {
			return
		}
		entity := firstEntityPos(body, run)
		if !entity.IsValid() {
			return
		}
		if loop, work := firstWorkLoopBefore(body, entity); loop != nil {
			pos := fset.Position(loop.Pos())
			findings = append(findings, Finding{
				RuleID:     "LOOP-001",
				Message:    "work loop runs before any Task/Group/Sequence; silent pre-output loops leave the run blank — put the loop inside Task.Define, or declare a Group first and give each item its own named Task",
				File:       filename,
				Line:       pos.Line,
				Column:     pos.Column,
				Suggestion: "declare the Task/Group/Sequence first, then run the loop inside task.Define(...) or for _, x := range items { group.Task(x).Define(...) }; move " + work + " into the task body",
			})
		}
	})
	return findings
}

// firstWorkLoopBefore is the earliest range loop in body, outside closures,
// that sits before limit and whose body makes an I/O or walk call.
func firstWorkLoopBefore(body *ast.BlockStmt, limit token.Pos) (*ast.RangeStmt, string) {
	var first *ast.RangeStmt
	work := ""
	inspectOutsideClosures(body, func(n ast.Node) {
		loop, ok := n.(*ast.RangeStmt)
		if !ok || loop.Pos() >= limit || (first != nil && loop.Pos() > first.Pos()) {
			return
		}
		if name := loopBodyWork(loop); name != "" {
			first, work = loop, name
		}
	})
	return first, work
}

// loopBodyWork names the first I/O or walk call inside loop's body.
func loopBodyWork(loop *ast.RangeStmt) string {
	name := ""
	ast.Inspect(loop.Body, func(n ast.Node) bool {
		if name == "" {
			name = loopWorkName(n)
		}
		return name == ""
	})
	return name
}
