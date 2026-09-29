// Package review — evo.Cause: Fail/Block are statement-form, so evo.Cause no
// longer affects the returned error the way it did before the 1.0 fold.
package review

import (
	"fmt"
	"go/ast"
	"go/token"
	"regexp"
	"slices"
	"strings"
)

// causeOptionPattern matches the shape `receiver.Fail("summary", evo.Cause(err))`
// (or Block) so a derived suggestion can name the exact rewrite the site
// should become, not just a generic pointer at the rule.
var causeOptionPattern = regexp.MustCompile(`(\w+)\.(Fail|Block)\(\s*"([^"]*)"\s*,\s*evo\.Cause\(([^()]*)\)\s*\)`)

// bareCausePattern catches every other evo.Cause( shape (backtick summary,
// extra options, wrong receiver text) so at least the deprecation itself is
// still flagged even when a derived returned-error rewrite isn't cheap.
var bareCausePattern = regexp.MustCompile(`evo\.Cause\(`)

// causeFindings flags evo.Cause: a Fail/Block(summary, evo.Cause(err)) site
// inside a function that returns error gets the exact `return`-based
// rewrite; the same shape outside any error-returning function (a plain
// helper called from a Define callback, not the callback itself) cannot
// safely suggest a bare `return` — that would either drop the Fail/Block
// resolution or fail to compile — so it gets a rewrite that keeps the
// resolving call and only wraps the error for the caller to return. Every
// other evo.Cause( is still flagged, once, with the generic message.
func causeFindings(in fileInput) []Finding {
	filename, src := in.filename, in.src
	var findings []Finding
	// derived marks every evo.Cause( the derived pass already covered.
	var derived []int
	for _, m := range causeOptionPattern.FindAllStringSubmatchIndex(src, -1) {
		recv, verb, summary, cause := src[m[2]:m[3]], src[m[4]:m[5]], src[m[6]:m[7]], src[m[8]:m[9]]
		if idx := strings.Index(src[m[0]:m[1]], "evo.Cause("); idx >= 0 {
			derived = append(derived, m[0]+idx)
		}
		var suggestion string
		switch {
		case !enclosingFuncReturnsError(in.file, in.fset, m[0]):
			// No safe `return` rewrite here: the enclosing function has no
			// error result, so a bare `return err`/`return fmt.Errorf(...)`
			// either drops the resolution or does not compile. Keep the
			// resolving call and have the caller return the error itself.
			suggestion = fmt.Sprintf(`%s.%s(%q); return %s (the resolving call has no error result here — have this function return %s so its caller can)`, recv, verb, summary, cause, cause)
		case verb == "Block":
			suggestion = fmt.Sprintf(`%s.Block(%q); return %s`, recv, summary, cause)
		default:
			suggestion = fmt.Sprintf(`return fmt.Errorf(%q, %s)`, summary+": %w", cause)
		}
		findings = append(findings, Finding{
			RuleID:     "API-032",
			Message:    "evo.Cause no longer affects the returned error since Fail/Block are statement-form; return a %w-wrapped error from Define instead",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: suggestion,
		})
	}
	for _, m := range bareCausePattern.FindAllStringIndex(src, -1) {
		if slices.Contains(derived, m[0]) {
			continue
		}
		suggestion := `replace evo.Cause(err) with return fmt.Errorf("...: %w", err) from Define, or keep the Block statement and return err`
		if !enclosingFuncReturnsError(in.file, in.fset, m[0]) {
			suggestion = `this function has no error result, so it cannot itself return a %w-wrapped error — keep the resolving Fail/Block call and have this function return the error for its caller to return from Define`
		}
		findings = append(findings, Finding{
			RuleID:     "API-032",
			Message:    "evo.Cause no longer affects the returned error since Fail/Block are statement-form; return a %w-wrapped error from Define instead",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: suggestion,
		})
	}
	return findings
}

// enclosingFuncReturnsError reports whether the innermost function literal
// or declaration whose body contains the byte offset off ends its result
// list in a bare `error`. A nil file (no parsed AST available) or an
// offset outside any function body reports false, since a bare `return
// err`/`return fmt.Errorf(...)` rewrite is unsafe to suggest without that
// guarantee.
func enclosingFuncReturnsError(file *ast.File, fset *token.FileSet, off int) bool {
	if file == nil || fset == nil {
		return false
	}
	tf := fset.File(file.Pos())
	if tf == nil || off < 0 || off > tf.Size() {
		return false
	}
	target := tf.Pos(off)

	var results *ast.FieldList
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		switch fn := n.(type) {
		case *ast.FuncDecl:
			if fn.Body != nil && fn.Body.Pos() <= target && target <= fn.Body.End() {
				results, found = fn.Type.Results, true
			}
		case *ast.FuncLit:
			if fn.Body != nil && fn.Body.Pos() <= target && target <= fn.Body.End() {
				results, found = fn.Type.Results, true
			}
		}
		return true
	})
	if !found || results == nil || len(results.List) == 0 {
		return false
	}
	last := results.List[len(results.List)-1]
	ident, ok := last.Type.(*ast.Ident)
	return ok && ident.Name == "error"
}
