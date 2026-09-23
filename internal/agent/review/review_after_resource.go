// Package review — API-053: a .After(...) edge whose only reason,
// corroborated by both a nearby comment and the two Tasks' own resource
// declarations, is shared-file/shared-resource exclusion rather than a real
// semantic dependency. ZYS-840's automatic resource claims (File/FSResource/
// LogicalResource) now serialize an overlapping write pair without any
// caller-declared edge, so an explicit .After kept only to avoid that race
// is redundant scheduling coupling that outlives its reason (ZYS-936).
package review

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// resourceContentionCommentSignals are the phrases a comment on or
// immediately above a .After(...) call uses when the edge's real (and only)
// job is avoiding a race on shared state, not sequencing a genuine producer/
// consumer step. This is deliberately a narrow, literal list — a comment
// that instead explains a real dependency ("needs the config Prepare
// writes") does not match and the rule stays silent.
var resourceContentionCommentSignals = []string{
	"same file", "shared file", "same resource", "shared resource",
	"same path", "shared path", "avoid race", "race condition",
	"contention", "exclusive access", "exclusion", "concurrent write",
	"avoid conflict", "serialize access",
}

// afterResourceEdge is one child.After(parent) call site with the comment
// text (if any) found on its line or the line immediately above.
type afterResourceEdge struct {
	afterEdge
	comment string
}

func collectAfterEdgesWithComments(src string, file *ast.File, fset *token.FileSet) []afterResourceEdge {
	lines := strings.Split(src, "\n")
	var out []afterResourceEdge
	for _, e := range collectAfterEdges(file) {
		out = append(out, afterResourceEdge{afterEdge: e, comment: nearbyLineComment(lines, e.pos, fset)})
	}
	return out
}

// nearbyLineComment returns the lower-cased text of a "//" comment on pos's
// own source line, or on the line directly above it when that line is a
// comment-only line — the two shapes a caller uses to explain why an
// .After(...) edge exists.
func nearbyLineComment(lines []string, pos token.Pos, fset *token.FileSet) string {
	lineNo := fset.Position(pos).Line
	if lineNo-1 < len(lines) {
		if idx := strings.Index(lines[lineNo-1], "//"); idx >= 0 {
			return strings.ToLower(lines[lineNo-1][idx+2:])
		}
	}
	if lineNo-2 >= 0 && lineNo-2 < len(lines) {
		prev := strings.TrimSpace(lines[lineNo-2])
		if after, ok := strings.CutPrefix(prev, "//"); ok {
			return strings.ToLower(after)
		}
	}
	return ""
}

// taskResourceLiterals maps each Task variable that declares a Define
// callback to the resource identity literals its callback body claims:
// evo.FSResource("path")/evo.LogicalResource("name") arguments, and
// evo.File(ctx, evo.FileSpec{Path: "path", ...})'s own auto-claimed Path.
// Both sides of an edge sharing one of these literals is the "resource
// declarations" leg of this rule's evidence (spec ZYS-840 Decisions
// 2026-09-23: File/FSResource/LogicalResource share one canonical claim
// namespace).
func taskResourceLiterals(file *ast.File, evoPkg string) map[string][]string {
	disproven := evoDisprovenVars(file)
	out := map[string][]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, fl, ok := funcLitArgAt(call, "Define", 1, 0, disproven)
		if !ok {
			return true
		}
		taskVar, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		ast.Inspect(fl.Body, func(n2 ast.Node) bool {
			inner, ok := n2.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch calledFuncDotted(inner) {
			case evoPkg + ".FSResource", evoPkg + ".LogicalResource":
				if len(inner.Args) == 1 {
					if lit, ok := stringLit(inner.Args[0]); ok {
						out[taskVar.Name] = append(out[taskVar.Name], lit)
					}
				}
			case evoPkg + ".File":
				if path, ok := fileSpecPathLiteral(inner, evoPkg); ok {
					out[taskVar.Name] = append(out[taskVar.Name], path)
				}
			}
			return true
		})
		return true
	})
	return out
}

// sharedLiteral reports the first literal both a and b claim in common.
func sharedLiteral(a, b []string) (string, bool) {
	set := map[string]bool{}
	for _, v := range a {
		set[v] = true
	}
	for _, v := range b {
		if set[v] {
			return v, true
		}
	}
	return "", false
}

// containsContentionSignal reports whether comment (already lower-cased)
// names one of resourceContentionCommentSignals.
func containsContentionSignal(comment string) bool {
	for _, s := range resourceContentionCommentSignals {
		if strings.Contains(comment, s) {
			return true
		}
	}
	return false
}

// detectAfterOnlyForResourceContention is API-053: an edge fires only when
// both legs of evidence line up — the child and parent Tasks' own Define
// bodies claim the identical resource literal (so Evo's automatic claim
// already serializes them), and the edge itself carries a comment naming
// exclusion, not a dependency, as the reason. Requiring both keeps this
// rule silent on EVO-DAG-003's real producer/consumer shape (a reader
// task with no resource claim of its own) and on undocumented edges this
// rule cannot safely explain.
func detectAfterOnlyForResourceContention(filename, src string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" {
		return nil
	}
	literals := taskResourceLiterals(file, pkg)
	var findings []Finding
	for _, e := range collectAfterEdgesWithComments(src, file, fset) {
		if !containsContentionSignal(e.comment) {
			continue
		}
		resource, ok := sharedLiteral(literals[e.child], literals[e.parent])
		if !ok {
			continue
		}
		pos := fset.Position(e.pos)
		findings = append(findings, Finding{
			RuleID:          "API-053",
			Severity:        "warning",
			Message:         e.child + ".After(" + e.parent + ") exists only to avoid a race on " + strconv.Quote(resource) + "; both Tasks already claim that resource, so Evo's automatic resource coordination already serializes them without this edge",
			File:            filename,
			Line:            pos.Line,
			Column:          pos.Column,
			Suggestion:      "delete " + e.child + ".After(" + e.parent + ") — the overlapping claim on " + strconv.Quote(resource) + " (File/FSResource/LogicalResource) already waits out the conflict; keep .After only for a real ordering dependency",
			RequiredVersion: dialectOneOne,
		})
	}
	return findings
}
