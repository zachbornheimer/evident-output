// Package review — EVO-PROVENANCE-001's narrow static detector (spec §57):
// a literal-only check that a function building an evo.Exec/evo.File call
// does not also visibly read, via a string-literal path, a file that call's
// Inputs omit. EVO-PROVENANCE-002 stays Detection: "guidance"
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

// filePathLiteral matches a string literal that is one plain path ending in
// an extension ("dir/template.txt"): no whitespace and no shell
// metacharacter, so a shell script operand ("cat a.txt > all.txt") is never
// mistaken for a path.
var filePathLiteral = regexp.MustCompile(`^[\w@%+=:,./~-]+\.[A-Za-z0-9]+$`)

// scriptFlags are the interpreter flags whose next Argv operand is program
// text, not a file ("sh -c SCRIPT", "perl -e SCRIPT").
var scriptFlags = map[string]bool{"-c": true, "-e": true}

// visibleReadFuncs are the calls this detector treats as "visibly reads
// these paths" (spec §57's own wording), keyed by package identifier —
// deliberately the cheapest, most common reads; anything else (bufio,
// io/fs, a third-party client) is a miss, not a guess. Every literal
// argument of os.ReadFile/Open/Stat and template.ParseFiles is a path read.
var visibleReadFuncs = map[string]map[string]bool{
	"os":       {"ReadFile": true, "Open": true, "Stat": true},
	"template": {"ParseFiles": true},
}

// pathCandidate is one literal path this detector considers, together with
// the source position to report if it turns out to be omitted.
type pathCandidate struct {
	path string
	pos  token.Pos
}

// inputClaims is what the evo.File/evo.Exec calls of one function body
// claim about freshness: the paths they list as Inputs, the paths they name
// but that are not inputs (Outputs, a File's Path, an Exec's program), and
// the literal Argv paths each Exec passes its child.
type inputClaims struct {
	spec      string // the spec type of the first claim, named in the suggestion
	inputs    map[string]bool
	notInputs map[string]bool
	argvPaths []pathCandidate
	hasClaims bool
}

// detectOmittedInputPath is EVO-PROVENANCE-001: inside one function body
// that also calls evo.File, or evo.Exec with a freshness claim, a
// string-literal path the same body visibly reads (os.ReadFile, os.Open,
// os.Stat, template.ParseFiles) or passes as a literal Exec Argv entry
// naming a file is flagged when no Inputs entry lists it. Literal-only and
// per-function: a variable path is never inferred, a nested function
// literal is its own body, and no Inputs entry is ever invented — the
// finding only names a path this exact source already spells out (see
// rules_provenance.go).
func detectOmittedInputPath(filename string, file *ast.File, fset *token.FileSet) []Finding {
	pkg := evoImportName(file)
	var findings []Finding
	forEachFuncBody(file, func(body *ast.BlockStmt) {
		claims := collectInputClaims(body, pkg)
		if !claims.hasClaims {
			return
		}
		for _, c := range claims.argvPaths {
			if !claims.inputs[c.path] && !claims.notInputs[c.path] {
				findings = append(findings, omittedInputFinding(filename, fset.Position(c.pos), c.path, claims.spec))
			}
		}
		walkOwnBody(body, func(n ast.Node) {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return
			}
			for _, c := range literalFileReads(call) {
				if !claims.inputs[c.path] && !claims.notInputs[c.path] {
					findings = append(findings, omittedInputFinding(filename, fset.Position(c.pos), c.path, claims.spec))
				}
			}
		})
	})
	return findings
}

// walkOwnBody visits every node of body except those inside a nested
// function literal, which forEachFuncBody visits as a body of its own.
func walkOwnBody(body *ast.BlockStmt, visit func(ast.Node)) {
	ast.Inspect(body, func(n ast.Node) bool {
		if lit, ok := n.(*ast.FuncLit); ok && lit.Body != body {
			return false
		}
		visit(n)
		return true
	})
}

// collectInputClaims gathers the freshness claims of the evo.File and
// evo.Exec calls in body. An evo.Exec that declares no Outputs, Inputs or
// Values makes no claim: with no Outputs it always runs.
func collectInputClaims(body *ast.BlockStmt, pkg string) inputClaims {
	claims := inputClaims{inputs: map[string]bool{}, notInputs: map[string]bool{}}
	walkOwnBody(body, func(n ast.Node) {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isEvoOperationCall(call, pkg) {
			return
		}
		spec := evoSpecCompositeLit(call)
		if spec == nil {
			return
		}
		isExec := call.Fun.(*ast.SelectorExpr).Sel.Name == "Exec"
		if isExec && compositeLitField(spec, "Outputs") == nil && compositeLitField(spec, "Inputs") == nil &&
			compositeLitField(spec, "Values") == nil {
			return
		}
		claims.addSpec(spec, isExec)
	})
	return claims
}

// addSpec records one File or Exec spec literal's claims.
func (c *inputClaims) addSpec(spec *ast.CompositeLit, isExec bool) {
	if !c.hasClaims {
		c.spec = "FileSpec"
		if isExec {
			c.spec = "ExecSpec"
		}
	}
	c.hasClaims = true
	for _, p := range stringLiteralElements(compositeLitField(spec, "Inputs")) {
		c.inputs[p] = true
	}
	// A declared Output names where the call writes, a File's Path what it
	// writes, and Argv[0] what program it runs; none is an undeclared input.
	for _, p := range stringLiteralElements(compositeLitField(spec, "Outputs")) {
		c.notInputs[p] = true
	}
	if path, ok := stringLiteralValue(compositeLitField(spec, "Path")); ok {
		c.notInputs[path] = true
	}
	if argv, ok := compositeLitField(spec, "Argv").(*ast.CompositeLit); ok && len(argv.Elts) > 0 {
		if program, ok := stringLiteralValue(argv.Elts[0]); ok {
			c.notInputs[program] = true
		}
		c.argvPaths = append(c.argvPaths, literalFileArgv(argv)...)
	}
}

// omittedInputFinding is the one finding shape both omission shapes (a
// visible read, a literal Exec Argv entry) report.
func omittedInputFinding(filename string, pos token.Position, path, spec string) Finding {
	return Finding{
		RuleID:     "EVO-PROVENANCE-001",
		Message:    fmt.Sprintf("Inputs omits %s, which this callback visibly reads", path),
		File:       filename,
		Line:       pos.Line,
		Column:     pos.Column,
		Suggestion: fmt.Sprintf("add %q to %s.Inputs so a change to it runs the operation again", path, spec),
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

// literalFileArgv extracts every string literal in an Argv composite
// literal after the program that is a plain file path (filePathLiteral),
// together with its position — a non-literal element (a variable) and the
// operand after a script flag (scriptFlags) are skipped, never inferred.
func literalFileArgv(argv *ast.CompositeLit) []pathCandidate {
	var out []pathCandidate
	afterScriptFlag := false
	for _, elt := range argv.Elts[1:] {
		s, ok := stringLiteralValue(elt)
		skip := afterScriptFlag
		afterScriptFlag = ok && scriptFlags[s]
		if !ok || skip || !filePathLiteral.MatchString(s) {
			continue
		}
		out = append(out, pathCandidate{path: s, pos: elt.Pos()})
	}
	return out
}

// literalFileReads reports the string-literal paths call reads when it is
// one of visibleReadFuncs. A variable argument contributes nothing — this
// detector never infers a path it cannot read directly from the source.
func literalFileReads(call *ast.CallExpr) []pathCandidate {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || !visibleReadFuncs[id.Name][sel.Sel.Name] {
		return nil
	}
	var out []pathCandidate
	for _, arg := range call.Args {
		if s, ok := stringLiteralValue(arg); ok {
			out = append(out, pathCandidate{path: s, pos: arg.Pos()})
		}
	}
	return out
}

// stringLiteralElements extracts every string-literal element of a
// []string{...} composite literal (e.g. ExecSpec.Outputs); a non-literal
// element contributes nothing rather than being guessed at. Returns nil if
// expr is nil or not a composite literal.
func stringLiteralElements(expr ast.Expr) []string {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var out []string
	for _, elt := range lit.Elts {
		if s, ok := stringLiteralValue(elt); ok {
			out = append(out, s)
		}
	}
	return out
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
