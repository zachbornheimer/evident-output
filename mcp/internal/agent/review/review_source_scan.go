// Package review — source scanning shared by the MCP teachability rule families
// (review_callback_resolution.go, review_fanout_wait.go, ...): callback
// discovery, call naming, substring signals, and lexeme masking.
package review

import (
	"go/ast"
	"go/scanner"
	"go/token"
	"strings"
)

// evoResolutionCallbacks collects every FuncLit passed directly as a
// Define callback — the shape whose return value resolves the task through
// evo's own scheduler (API-040/FP-006's scope).
func evoResolutionCallbacks(file *ast.File) []*ast.FuncLit {
	var out []*ast.FuncLit
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name == "Define" && len(call.Args) >= 1 {
			if fl, ok := call.Args[0].(*ast.FuncLit); ok {
				out = append(out, fl)
			}
		}
		return true
	})
	return out
}

// calledFuncName extracts the bare or method name a CallExpr invokes, for
// same-file call-graph lookups that stay honest about their limits (no
// cross-package resolution, no type checking of the receiver).
func calledFuncName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	default:
		return ""
	}
}

// containsAny reports whether s contains any of the given substrings.
func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// maskGoLexemes returns src with Go comments blanked out (replaced with
// spaces, newlines preserved) so token matching never fires on identifier-
// shaped text inside a comment. When alsoMaskStrings is true, string and
// rune literals are blanked too, for signals that must only match real Go
// syntax (an actual method call or type), never a mention of that text
// inside an unrelated string literal. The result has the same byte length
// and line breaks as src, so a byte offset found in the masked text is a
// valid offset into src for lineAt.
func maskGoLexemes(src string, alsoMaskStrings bool) string {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	var sc scanner.Scanner
	sc.Init(file, []byte(src), nil, scanner.ScanComments)
	out := []byte(src)
	for {
		pos, tok, lit := sc.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.COMMENT || (alsoMaskStrings && (tok == token.STRING || tok == token.CHAR)) {
			blankSpan(out, file.Offset(pos), len(lit))
		}
	}
	return string(out)
}

// blankSpan overwrites b[start:start+length] with spaces, leaving newlines
// untouched so line numbers computed from the result still match src.
func blankSpan(b []byte, start, length int) {
	for i := start; i < start+length && i >= 0 && i < len(b); i++ {
		if b[i] != '\n' {
			b[i] = ' '
		}
	}
}

// firstContainedToken returns the token from tokens that occurs earliest in
// s, and its byte offset within s (idx < 0 when none occurs; a tie goes to
// the token listed first) — used to pick a stable, meaningful finding line
// among several corroborating signals.
func firstContainedToken(s string, tokens []string) (token string, idx int) {
	best := -1
	for _, tok := range tokens {
		if i := strings.Index(s, tok); i >= 0 && (best < 0 || i < best) {
			best = i
			token = tok
		}
	}
	return token, best
}

func calledFuncDotted(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return exprDottedName(fn.X) + "." + fn.Sel.Name
	default:
		return ""
	}
}
