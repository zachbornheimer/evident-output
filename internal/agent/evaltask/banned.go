package evaltask

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Banned-pattern detector IDs, one per recurring bad answer.
const (
	PatternOuterHandoff     = "outer-mutable-handoff"
	PatternGoroutine        = "goroutine-concurrency"
	PatternRedundantAfter   = "redundant-after-in-sequence"
	PatternGiantLoopTask    = "giant-loop-task"
	PatternPresentationTask = "presentation-only-task"
	PatternDeclareInDefine  = "declare-in-callback"
	PatternLocalContainer   = "local-container-interface"
)

const (
	methodDefine   = "Define"
	methodCompute  = "Compute"
	methodTask     = "Task"
	methodAfter    = "After"
	methodSequence = "Sequence"
	methodGroup    = "Group"
)

var presentationTaskName = regexp.MustCompile(`(?i)\b(summary|overview|banner)\b`)

// detector reports whether one parsed file contains its pattern.
type detector func(file *ast.File) bool

var detectors = map[string]detector{
	PatternOuterHandoff:     hasOuterHandoff,
	PatternGoroutine:        hasGoroutine,
	PatternRedundantAfter:   hasRedundantSequenceAfter,
	PatternGiantLoopTask:    hasLoopInTaskCallback,
	PatternPresentationTask: hasPresentationTask,
	PatternDeclareInDefine:  hasDeclarationInCallback,
	PatternLocalContainer:   hasLocalContainerInterface,
}

// KnownPatterns lists every detector ID, sorted.
func KnownPatterns() []string {
	ids := make([]string, 0, len(detectors))
	for id := range detectors {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// FindBannedPatterns parses every Go file in candidate and returns which of
// the named detectors fire, sorted. An unknown detector ID is an error so a
// typo in expect.json cannot silently disable a check.
func FindBannedPatterns(candidate fs.FS, patterns []string) ([]string, error) {
	files, err := parseCandidate(candidate)
	if err != nil {
		return nil, err
	}
	var hits []string
	for _, id := range patterns {
		detect, ok := detectors[id]
		if !ok {
			return nil, fmt.Errorf("unknown banned pattern %q (known: %s)", id, strings.Join(KnownPatterns(), ", "))
		}
		if slices.ContainsFunc(files, detect) {
			hits = append(hits, id)
		}
	}
	sort.Strings(hits)
	return hits, nil
}

func parseCandidate(candidate fs.FS) ([]*ast.File, error) {
	names, err := fs.Glob(candidate, "*"+candidateSuffix)
	if err != nil {
		return nil, fmt.Errorf("list candidate Go files: %w", err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range names {
		src, err := fs.ReadFile(candidate, name)
		if err != nil {
			return nil, fmt.Errorf("read candidate file %s: %w", name, err)
		}
		file, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parse candidate file %s: %w", name, err)
		}
		files = append(files, file)
	}
	return files, nil
}

func hasGoroutine(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.GoStmt:
			found = true
		case *ast.SelectorExpr:
			if qualified(node, "sync", "WaitGroup") || qualified(node, "errgroup", "Group") {
				found = true
			}
		}
		return !found
	})
	return found
}

func qualified(sel *ast.SelectorExpr, pkg, name string) bool {
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == pkg && sel.Sel.Name == name
}

func hasPresentationTask(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if ok && calledMethod(call) == methodTask && len(call.Args) > 0 {
			if lit, isLit := call.Args[0].(*ast.BasicLit); isLit && presentationTaskName.MatchString(lit.Value) {
				found = true
			}
		}
		return !found
	})
	return found
}

func hasLocalContainerInterface(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		if _, isInterface := spec.Type.(*ast.InterfaceType); isInterface && strings.Contains(spec.Name.Name, "Container") {
			found = true
		}
		return !found
	})
	return found
}

// calledMethod is the selector name of a call (x.Task(...) -> "Task"), or
// "" when the callee is not a selector.
func calledMethod(call *ast.CallExpr) string {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		return sel.Sel.Name
	}
	return ""
}
