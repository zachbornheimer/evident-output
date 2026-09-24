package adopt

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// inventoryFile parses one Go file and returns every non-evo output site it
// recognizes: spinner/progress-bar imports and fmt/log/os call sites that
// print, exit, or panic outside evo's own front doors.
func inventoryFile(fset *token.FileSet, path string, src []byte) ([]Finding, *ast.File, error) {
	f, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, nil, err
	}

	findings := spinnerFindings(fset, path, f)
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if find, ok := classifyCall(fset, path, call); ok {
			findings = append(findings, find)
		}
		return true
	})
	return findings, f, nil
}

func spinnerFindings(fset *token.FileSet, path string, f *ast.File) []Finding {
	var findings []Finding
	for _, imp := range f.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		suggestion, ok := spinnerImports[importPath]
		if !ok {
			continue
		}
		pos := fset.Position(imp.Pos())
		findings = append(findings, Finding{
			File:       path,
			Line:       pos.Line,
			Pattern:    "import " + importPath,
			Rung:       RungTaskDefine,
			Suggestion: suggestion,
			Certainty:  CertaintyHigh,
		})
	}
	return findings
}

// classifyCall recognizes one non-evo output call site. It returns ok=false
// for anything it does not have a specific, deterministic rule for —
// silence, not a guess, is the correct answer for an unrecognized call.
func classifyCall(fset *token.FileSet, path string, call *ast.CallExpr) (Finding, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return Finding{}, false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return Finding{}, false
	}
	site := callSite{fset: fset, path: path, call: call, pattern: pkg.Name + "." + sel.Sel.Name}

	switch pkg.Name {
	case "os":
		return classifyOSCall(site, sel.Sel.Name)
	case "log":
		return classifyLogCall(site, sel.Sel.Name)
	case "fmt":
		return classifyFmtCall(site, sel.Sel.Name)
	default:
		return Finding{}, false
	}
}

// callSite carries the shared context every classify* helper needs so each
// one reads as a short lookup table, not a re-derivation of position/pattern.
type callSite struct {
	fset    *token.FileSet
	path    string
	call    *ast.CallExpr
	pattern string
}

func (s callSite) finding(rung Rung, suggestion string, certainty Certainty) Finding {
	pos := s.fset.Position(s.call.Pos())
	return Finding{
		File: s.path, Line: pos.Line, Pattern: s.pattern,
		Rung: rung, Suggestion: suggestion, Certainty: certainty,
	}
}

func classifyOSCall(s callSite, method string) (Finding, bool) {
	if method != "Exit" {
		return Finding{}, false
	}
	return s.finding(RungInitMain,
		"let evo.Main derive the exit code (0/1/2/130) via os.Exit(evo.Main(run)) — return the error from run(ctx) instead of calling os.Exit directly.",
		CertaintyHigh,
	), true
}

func classifyLogCall(s callSite, method string) (Finding, bool) {
	switch method {
	case "Fatal", "Fatalf", "Fatalln", "Panic", "Panicf", "Panicln":
		return s.finding(RungInitMain,
			"this exits/panics directly, bypassing evo's exit-code contract — resolve the active Task with Fail/Failf or Block/Blockf and return, then let os.Exit(evo.Main(run)) derive the exit code.",
			CertaintyHigh,
		), true
	case "Print", "Printf", "Println":
		return s.finding(RungTaskDefine,
			"replace with evo.Println/Print/Printf for a durable note, or task.Doing plus the Task's Define callback if this reports lifecycle state — see the common-api guide.",
			CertaintyNeedsReview,
		), true
	default:
		return Finding{}, false
	}
}

func classifyFmtCall(s callSite, method string) (Finding, bool) {
	switch method {
	case "Print", "Printf", "Println":
		return s.finding(RungTaskDefine,
			"replace with evo.Println/Print/Printf (durable notes) or a Task's Doing and Define — never fmt.Print* while a live region may be open.",
			CertaintyNeedsReview,
		), true
	case "Fprint", "Fprintf", "Fprintln":
		if len(s.call.Args) == 0 {
			return Finding{}, false
		}
		if isOsStdout(s.call.Args[0]) {
			s.pattern += "(os.Stdout, ...)"
			return s.finding(RungTaskDefine,
				"writing os.Stdout directly bypasses evo's live region — route through evo.Init(Config{Stdout: os.Stdout}) and evo.Println/Task instead.",
				CertaintyNeedsReview,
			), true
		}
		if sink, ok := injectedWriterSinkName(s.call.Args[0]); ok {
			s.pattern += "(" + sink + ", ...)"
			return s.finding(RungTaskDefine,
				"writing an injected stdout/stderr sink bypasses evo's live region the same way os.Stdout would — route it through evo.Init(Config{Stdout: ...}) and evo.Println/Task instead.",
				CertaintyNeedsReview,
			), true
		}
		return Finding{}, false
	default:
		return Finding{}, false
	}
}

func isOsStdout(arg ast.Expr) bool {
	sel, ok := arg.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "os" && sel.Sel.Name == "Stdout"
}

// injectedSinkFieldNames is the small, deliberately narrow set of
// conventional names an injected stdout/stderr sink carries — a struct
// field (homelabctl's runtime.stdout/stderr) or a bare parameter — matched
// case-insensitively. It excludes generic names like "out" or "err" that
// collide too often with unrelated values (an error variable is almost
// always named "err") to be a reliable signal on their own.
var injectedSinkFieldNames = map[string]bool{
	"stdout": true,
	"stderr": true,
}

// injectedWriterSinkName reports whether arg is a struct-field access or
// bare identifier named for an injected stdout/stderr sink — the shape
// fmt.Fprintf(rt.stdout, ...) or fmt.Fprintln(stderr, ...) takes when a
// program wires its own io.Writer field instead of using os.Stdout/
// os.Stderr directly (see ZYS-1018, homelabctl's cmd/homelabctl/runtime.go).
// This is a selector-name heuristic, the same kind detectFacades already
// uses for writer fields — it cannot see through renamed fields or fields
// whose name doesn't match the convention.
func injectedWriterSinkName(arg ast.Expr) (string, bool) {
	switch e := arg.(type) {
	case *ast.SelectorExpr:
		if injectedSinkFieldNames[strings.ToLower(e.Sel.Name)] {
			if base, ok := e.X.(*ast.Ident); ok {
				return base.Name + "." + e.Sel.Name, true
			}
		}
	case *ast.Ident:
		if injectedSinkFieldNames[strings.ToLower(e.Name)] {
			return e.Name, true
		}
	}
	return "", false
}
