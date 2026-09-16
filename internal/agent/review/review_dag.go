// Package review — spec §57 EVO-DAG-001/002/003: application code inventing
// its own scheduling (a goroutine, a hand-chained .After(...)) or leaving a
// visible producer/consumer relationship unordered, where Group/Sequence/
// After already say the same thing declaratively.
package review

import (
	"go/ast"
	"go/token"
	"strings"
)

// ===== EVO-DAG-001: a goroutine wraps a call that already submits work to
// Evo's scheduler (.Define). Group/Sequence already run eligible children
// concurrently; the goroutine exists only to make Evo Tasks parallel a
// second time (spec §4, §57).

func detectGoroutineWrappingDefine(filename, src string) []Finding {
	var findings []Finding
	for _, marker := range fanOutClosureMarkers {
		for i := 0; i < len(src); {
			idx := strings.Index(src[i:], marker)
			if idx < 0 {
				break
			}
			idx += i
			body, start, ok := balancedBraceBody(src, idx)
			if !ok {
				break
			}
			if containsAnyMarker(body, []string{".Define("}) {
				findings = append(findings, Finding{
					RuleID:          "EVO-DAG-001",
					Severity:        "warning",
					Message:         "a goroutine wraps a call that already submits work to Evo's scheduler (.Define); Group/Sequence already run eligible children concurrently",
					File:            filename,
					Line:            lineAt(src, start),
					Suggestion:      "delete the goroutine and call task.Define(...) directly — Group already schedules independent Tasks concurrently",
					RequiredVersion: evoDagEvidenceRequiredVersion,
				})
			}
			i = start + len(body)
		}
	}
	return findings
}

// ===== EVO-DAG-002: a chain of .After(...) calls reproduces exactly the
// ordering evo.Sequence already gives its children automatically (spec §5,
// §6, §57).

// afterEdge is one child.After(parent) call site.
type afterEdge struct {
	child, parent string
	pos           token.Pos
}

func collectAfterEdges(file *ast.File) []afterEdge {
	var edges []afterEdge
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "After" || len(call.Args) != 1 || !isLikelyEvoReceiver(sel.X) {
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

// findAfterChain reports the first edge whose child is itself another
// edge's parent — a dependency chain two levels deep, the exact shape
// Sequence already expresses without any explicit .After (a single
// exceptional edge, spec §6's own database.After(network), stays silent).
func findAfterChain(edges []afterEdge) (token.Pos, bool) {
	parents := map[string]bool{}
	for _, e := range edges {
		parents[e.parent] = true
	}
	for _, e := range edges {
		if parents[e.child] {
			return e.pos, true
		}
	}
	return 0, false
}

func hasAfterEdge(edges []afterEdge, child, parent string) bool {
	for _, e := range edges {
		if e.child == child && e.parent == parent {
			return true
		}
	}
	return false
}

func detectAfterChainDuplicatesSequence(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pos, ok := findAfterChain(collectAfterEdges(file))
	if !ok {
		return nil
	}
	return []Finding{{
		RuleID:          "EVO-DAG-002",
		Severity:        "warning",
		Message:         "a chain of .After(...) calls reproduces the exact ordering evo.Sequence already gives its children automatically",
		File:            filename,
		Line:            fset.Position(pos).Line,
		Column:          fset.Position(pos).Column,
		Suggestion:      "replace the chained .After(...) calls with one evo.Sequence(name) and declare each Task as seq.Task(...) in order",
		RequiredVersion: evoDagEvidenceRequiredVersion,
	}}
}

// ===== EVO-DAG-003: a producer/consumer resource relationship is visible
// (one Task's evo.File(...) establishes a path; another Task's raw read
// call observes the same literal path) but nothing orders them — first-run
// scheduler ordering is missing (spec §11.5, §47, §57).

// resourceEdge is one file path a Task's Define callback either establishes
// (evo.File's Path field) or reads (a raw os.ReadFile/os.Open/ioutil.ReadFile
// call) — DAG-003's producer/consumer signal.
type resourceEdge struct {
	taskVar string
	path    string
	pos     token.Pos
}

var resourceReadCallNames = map[string]bool{
	"os.ReadFile": true, "os.Open": true, "ioutil.ReadFile": true,
}

func collectResourceEdges(file *ast.File, evoPkg string) (produces, consumes []resourceEdge) {
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		_, fl, ok := funcLitArgAt(call, "Define", 1, 0)
		if !ok {
			return true
		}
		sel := call.Fun.(*ast.SelectorExpr)
		taskVar, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		ast.Inspect(fl.Body, func(n2 ast.Node) bool {
			inner, ok := n2.(*ast.CallExpr)
			if !ok {
				return true
			}
			if path, ok := fileSpecPathLiteral(inner, evoPkg); ok {
				produces = append(produces, resourceEdge{taskVar.Name, path, inner.Pos()})
				return true
			}
			if resourceReadCallNames[calledFuncDotted(inner)] && len(inner.Args) >= 1 {
				if path, ok := stringLit(inner.Args[0]); ok {
					consumes = append(consumes, resourceEdge{taskVar.Name, path, inner.Pos()})
				}
			}
			return true
		})
		return true
	})
	return produces, consumes
}

// fileSpecPathLiteral extracts the Path field's string literal from an
// evo.File(ctx, evo.FileSpec{Path: "...", ...}) call.
func fileSpecPathLiteral(call *ast.CallExpr, evoPkg string) (string, bool) {
	if calledFuncDotted(call) != evoPkg+".File" || len(call.Args) < 2 {
		return "", false
	}
	lit, ok := call.Args[1].(*ast.CompositeLit)
	if !ok {
		return "", false
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Path" {
			return stringLit(kv.Value)
		}
	}
	return "", false
}

func detectMissingProducerConsumerOrdering(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" {
		return nil
	}
	produces, consumes := collectResourceEdges(file, pkg)
	edges := collectAfterEdges(file)

	var findings []Finding
	for _, c := range consumes {
		for _, p := range produces {
			if p.path != c.path || p.taskVar == c.taskVar {
				continue
			}
			if hasAfterEdge(edges, c.taskVar, p.taskVar) {
				continue
			}
			findings = append(findings, Finding{
				RuleID:          "EVO-DAG-003",
				Severity:        "warning",
				Message:         "task " + c.taskVar + " reads " + c.path + ", which task " + p.taskVar + " produces, but nothing orders them; first-run scheduling gives no guarantee " + p.taskVar + " already ran",
				File:            filename,
				Line:            fset.Position(c.pos).Line,
				Column:          fset.Position(c.pos).Column,
				Suggestion:      c.taskVar + ".After(" + p.taskVar + "), or declare both under one evo.Sequence so " + p.taskVar + " always runs first",
				RequiredVersion: evoDagEvidenceRequiredVersion,
			})
		}
	}
	return findings
}
