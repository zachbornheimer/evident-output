// Package review — API-056: a .After(...) edge whose only reason,
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
	"go/types"
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

// resourceClaim is one resource identity literal a Task's Define callback
// claims, together with a write-shape signature that captures whether the
// claim's own outcome is order-independent. A bare FSResource/LogicalResource
// claim (no Effect wrapper) and an evo.File claim with no Contents field
// both signature as "" — there is nothing in the call to say the two writes
// differ, so they default to order-independent, matching this rule's
// original behavior. Once a claim's write is visible (a File's Contents
// expression, or an Effect's Verb), two claims on the same literal must
// also match on signature: differing Contents (cfg vs warm) or conflicting
// Verbs (Update vs Delete) prove the write's final state, or the resource's
// final existence, depends on which Task runs last — a real ordering
// dependency a resource claim's mutual exclusion does not resolve (ZYS-840
// Decisions: "a resource claim only coordinates overlap; it does not create
// an After dependency"). This rule fires only when it can positively show
// the overlap is order-invariant, never merely because a comment says so.
type resourceClaim struct {
	literal   string
	signature string
}

// taskResourceClaims maps each Task variable that declares a Define
// callback to the resourceClaims its callback body makes: evo.FSResource/
// evo.LogicalResource arguments (bare, or via evo.Effect's Resource field),
// and evo.File(ctx, evo.FileSpec{Path: "path", ...})'s own auto-claimed
// Path. Both sides of an edge sharing one of these claims is the "resource
// declarations" leg of this rule's evidence (spec ZYS-840 Decisions
// 2026-09-23: File/FSResource/LogicalResource share one canonical claim
// namespace).
func taskResourceClaims(file *ast.File, evoPkg string) map[string][]resourceClaim {
	disproven := evoDisprovenVars(file)
	out := map[string][]resourceClaim{}
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
			case evoPkg + ".Effect":
				if literal, signature, ok := effectResourceClaim(inner, evoPkg); ok {
					out[taskVar.Name] = append(out[taskVar.Name], resourceClaim{literal: literal, signature: signature})
				}
				return false
			case evoPkg + ".FSResource", evoPkg + ".LogicalResource":
				if len(inner.Args) == 1 {
					if lit, ok := stringLit(inner.Args[0]); ok {
						out[taskVar.Name] = append(out[taskVar.Name], resourceClaim{literal: lit})
					}
				}
			case evoPkg + ".File":
				if path, ok := fileSpecPathLiteral(inner, evoPkg); ok {
					out[taskVar.Name] = append(out[taskVar.Name], resourceClaim{
						literal:   path,
						signature: fileSpecContentsSignature(inner, evoPkg),
					})
				}
			}
			return true
		})
		return true
	})
	return out
}

// effectResourceClaim extracts, from an evo.Effect(ctx, evo.EffectSpec{...},
// fn) call, the Resource field's underlying FSResource/LogicalResource
// literal and the Verb field's source text as the write-shape signature.
func effectResourceClaim(call *ast.CallExpr, evoPkg string) (literal, signature string, ok bool) {
	if len(call.Args) < 2 {
		return "", "", false
	}
	spec, isLit := call.Args[1].(*ast.CompositeLit)
	if !isLit {
		return "", "", false
	}
	for _, elt := range spec.Elts {
		kv, isKV := elt.(*ast.KeyValueExpr)
		if !isKV {
			continue
		}
		key, isIdent := kv.Key.(*ast.Ident)
		if !isIdent {
			continue
		}
		switch key.Name {
		case "Resource":
			inner, isCall := kv.Value.(*ast.CallExpr)
			if !isCall {
				continue
			}
			switch calledFuncDotted(inner) {
			case evoPkg + ".FSResource", evoPkg + ".LogicalResource":
				if len(inner.Args) == 1 {
					if s, ok := stringLit(inner.Args[0]); ok {
						literal = s
					}
				}
			}
		case "Verb":
			signature = types.ExprString(kv.Value)
		}
	}
	return literal, signature, literal != ""
}

// fileSpecContentsSignature extracts the Contents field's source text from
// an evo.File(ctx, evo.FileSpec{...}) call, or "" when the call has no
// Contents field (or is not an evo.File call).
func fileSpecContentsSignature(call *ast.CallExpr, evoPkg string) string {
	if calledFuncDotted(call) != evoPkg+".File" || len(call.Args) < 2 {
		return ""
	}
	lit, ok := call.Args[1].(*ast.CompositeLit)
	if !ok {
		return ""
	}
	for _, elt := range lit.Elts {
		kv, isKV := elt.(*ast.KeyValueExpr)
		if !isKV {
			continue
		}
		if key, isIdent := kv.Key.(*ast.Ident); isIdent && key.Name == "Contents" {
			return types.ExprString(kv.Value)
		}
	}
	return ""
}

// sharedClaim reports the literal of the first claim both a and b make in
// common — matching on literal AND signature, so a shared path/name with
// differing write shapes (differing Contents, conflicting Verbs) is never
// reported as order-independent.
func sharedClaim(a, b []resourceClaim) (string, bool) {
	for _, ca := range a {
		for _, cb := range b {
			if ca.literal != "" && ca.literal == cb.literal && ca.signature == cb.signature {
				return ca.literal, true
			}
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

// detectAfterOnlyForResourceContention is API-056: an edge fires only when
// three legs of evidence line up — the child and parent Tasks' own Define
// bodies claim the identical resource literal with the identical write
// shape (so the overlap is order-invariant, not just mutually exclusive),
// the edge itself carries a comment naming exclusion, not a dependency, as
// the reason. Requiring all three keeps this rule silent on EVO-DAG-003's
// real producer/consumer shape (a reader task with no resource claim of its
// own), on undocumented edges this rule cannot safely explain, and on a
// shared claim whose writes conflict (differing File Contents, conflicting
// Effect Verbs) — a case where deleting .After would leave the outcome
// nondeterministic rather than merely redundant (ZYS-936).
func detectAfterOnlyForResourceContention(filename, src string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	if pkg == "" {
		return nil
	}
	claims := taskResourceClaims(file, pkg)
	var findings []Finding
	for _, e := range collectAfterEdgesWithComments(src, file, fset) {
		if !containsContentionSignal(e.comment) {
			continue
		}
		resource, ok := sharedClaim(claims[e.child], claims[e.parent])
		if !ok {
			continue
		}
		pos := fset.Position(e.pos)
		findings = append(findings, Finding{
			RuleID:          "API-056",
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
