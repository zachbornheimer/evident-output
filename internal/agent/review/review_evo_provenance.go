// Package review — EVO-PROVENANCE-001's narrow static detector (spec §57):
// a literal-only check that a function building an evo.Exec/evo.File call
// with a Basis does not also visibly read, via a string-literal path, a
// file its own Basis omits. EVO-PROVENANCE-002 stays Detection: "guidance"
// (see rules_provenance.go) — distinguishing a legitimate cached-but-
// reverified Verify from one that trusts an opaque manifest alone needs
// call-site intent no AST shape carries.
package review

import (
	"fmt"
	"go/ast"
	"go/token"
	"regexp"
)

// fileLiteralSuffix matches a string literal that plausibly names a file
// ("...template.txt") rather than a bare flag or directory — the same
// literal-only signal used for both read-call and Exec-Arg detection.
var fileLiteralSuffix = regexp.MustCompile(`\.[A-Za-z0-9]+$`)

// omittedBasisPathReadFuncs are the os package calls this detector treats
// as "visibly reads a path" (spec §57's own wording) — deliberately the
// three cheapest, most common single-path reads; anything else (bufio,
// io/fs, a third-party client) is a miss, not a guess.
var omittedBasisPathReadFuncs = map[string]bool{"ReadFile": true, "Open": true, "Stat": true}

// basisPathCandidate is one literal path this detector considers, together
// with the source position to report if it turns out to be omitted.
type basisPathCandidate struct {
	path string
	pos  token.Pos
}

// detectOmittedBasisPath is EVO-PROVENANCE-001: inside one function body
// that also calls evo.Exec/evo.File with a Basis, a string-literal path the
// same body visibly reads (os.ReadFile/os.Open/os.Stat) or passes as a
// literal Exec Arg naming a file is flagged when no Basis entry lists it
// via evo.FSPath. Literal-only and per-call: a variable path is never
// inferred, and no Basis entry is invented — the finding only ever names a
// path this exact source already spells out (see rules_provenance.go).
func detectOmittedBasisPath(filename string, file *ast.File, fset *token.FileSet) []Finding {
	var findings []Finding
	forEachFuncBody(file, func(body *ast.BlockStmt) {
		basisPaths := map[string]bool{}
		var execArgCandidates []basisPathCandidate
		hasBasisSpec := false

		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isEvoOperationCall(call) {
				return true
			}
			spec := evoSpecCompositeLit(call)
			if spec == nil {
				return true
			}
			basisExpr := compositeLitField(spec, "Basis")
			if basisExpr == nil {
				return true
			}
			hasBasisSpec = true
			for _, p := range fsPathLiterals(basisExpr) {
				basisPaths[p] = true
			}
			if argsExpr := compositeLitField(spec, "Args"); argsExpr != nil {
				execArgCandidates = append(execArgCandidates, literalFileArgs(argsExpr)...)
			}
			return true
		})
		if !hasBasisSpec {
			return
		}

		for _, c := range execArgCandidates {
			if basisPaths[c.path] {
				continue
			}
			findings = append(findings, omittedBasisFinding(filename, fset.Position(c.pos), c.path))
		}

		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			path, pos, ok := literalFileReadArg(call)
			if !ok || basisPaths[path] {
				return true
			}
			findings = append(findings, omittedBasisFinding(filename, fset.Position(pos), path))
			return true
		})
	})
	return findings
}

// omittedBasisFinding is the one finding shape both omission shapes (a
// visible read, a literal Exec Arg) report.
func omittedBasisFinding(filename string, pos token.Position, path string) Finding {
	return Finding{
		RuleID:     "EVO-PROVENANCE-001",
		Severity:   "warning",
		Message:    fmt.Sprintf("Basis omits %s, which this callback visibly reads", path),
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: fmt.Sprintf("Add evo.FSPath(%q) to Basis", path),
	}
}

// evoSpecCompositeLit returns the ExecSpec/FileSpec composite literal
// passed to an evo.Exec/evo.File call, or nil if call passes none directly
// (e.g. a pre-built variable, which this literal-only detector never
// inspects).
func evoSpecCompositeLit(call *ast.CallExpr) *ast.CompositeLit {
	for _, arg := range call.Args {
		if lit, ok := arg.(*ast.CompositeLit); ok {
			return lit
		}
	}
	return nil
}

// compositeLitField returns the value expression for a keyed field in a
// struct composite literal, or nil if absent.
func compositeLitField(lit *ast.CompositeLit, name string) ast.Expr {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if id, ok := kv.Key.(*ast.Ident); ok && id.Name == name {
			return kv.Value
		}
	}
	return nil
}

// fsPathLiterals extracts every literal path an []evo.Fingerprint{...}
// Basis composite literal names via evo.FSPath("..."); a non-literal
// argument (a variable, a formatted expression) contributes nothing rather
// than being guessed at.
func fsPathLiterals(expr ast.Expr) []string {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var paths []string
	for _, elt := range lit.Elts {
		call, ok := elt.(*ast.CallExpr)
		if !ok {
			continue
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || id.Name != "evo" || sel.Sel.Name != "FSPath" || len(call.Args) == 0 {
			continue
		}
		if path, ok := stringLiteralValue(call.Args[0]); ok {
			paths = append(paths, path)
		}
	}
	return paths
}

// literalFileArgs extracts every string literal in an []string{...} Exec
// Args composite literal that plausibly names a file (fileLiteralSuffix),
// together with its position — a non-literal element (a variable) is
// skipped, never inferred.
func literalFileArgs(expr ast.Expr) []basisPathCandidate {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var out []basisPathCandidate
	for _, elt := range lit.Elts {
		s, ok := stringLiteralValue(elt)
		if !ok || !fileLiteralSuffix.MatchString(s) {
			continue
		}
		out = append(out, basisPathCandidate{path: s, pos: elt.Pos()})
	}
	return out
}

// literalFileReadArg reports whether call is os.ReadFile/os.Open/os.Stat
// applied to a string-literal path, returning that literal path and its
// position. A variable argument reports ok=false — this detector never
// infers a path it cannot read directly from the source.
func literalFileReadArg(call *ast.CallExpr) (path string, pos token.Pos, ok bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", 0, false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != "os" || !omittedBasisPathReadFuncs[sel.Sel.Name] || len(call.Args) == 0 {
		return "", 0, false
	}
	s, ok := stringLiteralValue(call.Args[0])
	if !ok {
		return "", 0, false
	}
	return s, call.Args[0].Pos(), true
}

// stringLiteralValue unquotes expr if it is a string literal.
func stringLiteralValue(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconvUnquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}
