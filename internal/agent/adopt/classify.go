package adopt

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
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

// identBinding resolves what one identifier in a function's parameter or
// receiver list is declared as, syntactically (no go/types): either a
// direct io.Writer, a named struct type (to chase a field access like
// rt.stdout back to that struct's field types), or a named interface type
// declared in the same package directory (to chase a call like
// deps.Docker.PullImage back to the interface it's declared against). Only
// one of the three is ever set.
type identBinding struct {
	writerDirect  bool
	typeName      string
	interfaceType string
}

// bindingsFor maps every receiver and parameter name of decl to its
// identBinding, so a call site inside decl's body can resolve rt.stdout or
// deps.Docker without re-deriving decl's signature at every call.
// interfaceTypes classifies a named type as interfaceType instead of
// typeName; pass nil when the caller only needs typeName/writerDirect.
func bindingsFor(decl *ast.FuncDecl, dir string, interfaceTypes map[string]bool) map[string]identBinding {
	bindings := map[string]identBinding{}
	add := func(field *ast.Field) {
		binding, ok := bindingFromType(field.Type, dir, interfaceTypes)
		if !ok {
			return
		}
		for _, name := range field.Names {
			bindings[name.Name] = binding
		}
	}
	if decl.Recv != nil {
		for _, field := range decl.Recv.List {
			add(field)
		}
	}
	if decl.Type.Params != nil {
		for _, field := range decl.Type.Params.List {
			add(field)
		}
	}
	return bindings
}

func bindingFromType(expr ast.Expr, dir string, interfaceTypes map[string]bool) (identBinding, bool) {
	if isIOWriterType(expr) {
		return identBinding{writerDirect: true}, true
	}
	name, ok := resolveNamedType(expr)
	if !ok {
		return identBinding{}, false
	}
	if interfaceTypes[dir+"."+name] {
		return identBinding{interfaceType: name}, true
	}
	return identBinding{typeName: name}, true
}

// resolveNamedType returns the identifier name a (possibly pointer) named
// type expression is declared with — never an external package's qualified
// type (io.Writer, strings.Builder), which is exactly the exclusion
// ZYS-1018/1019 need: a field or call resolved to an out-of-package type
// can't be chased into its declaration syntactically, so it's correctly
// left unmatched rather than guessed at.
func resolveNamedType(expr ast.Expr) (string, bool) {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return resolveNamedType(t.X)
	case *ast.Ident:
		return t.Name, true
	default:
		return "", false
	}
}

// writerFieldIndex maps "dir.TypeName" to the set of that struct's field
// names declared as io.Writer — built once across every parsed file so a
// struct and the function that reads its field can live in different files
// of the same package (see ZYS-1018).
type writerFieldIndex map[string]map[string]bool

func buildWriterFieldIndex(files []parsedFile) writerFieldIndex {
	idx := writerFieldIndex{}
	for _, pf := range files {
		dir := filepath.Dir(pf.Path)
		ast.Inspect(pf.File, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := spec.Type.(*ast.StructType)
			if !ok || st.Fields == nil {
				return true
			}
			key := dir + "." + spec.Name.Name
			for _, field := range st.Fields.List {
				if !isIOWriterType(field.Type) {
					continue
				}
				for _, name := range field.Names {
					if idx[key] == nil {
						idx[key] = map[string]bool{}
					}
					idx[key][name.Name] = true
				}
			}
			return true
		})
	}
	return idx
}

// detectWriterSinkFindings is the second pass ZYS-1018 needs: a fmt.Fprint*
// call whose first argument resolves — by declared type, not by name — to
// an io.Writer, whether that's a bare io.Writer parameter or a struct
// field typed io.Writer. It runs after every file is parsed so a struct
// declared in one file and used in another still resolves.
func detectWriterSinkFindings(fset *token.FileSet, files []parsedFile) []Finding {
	fields := buildWriterFieldIndex(files)
	var findings []Finding
	for _, pf := range files {
		dir := filepath.Dir(pf.Path)
		ast.Inspect(pf.File, func(n ast.Node) bool {
			decl, ok := n.(*ast.FuncDecl)
			if !ok || decl.Body == nil {
				return true
			}
			bindings := bindingsFor(decl, dir, nil)
			ast.Inspect(decl.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "fmt" {
					return true
				}
				switch sel.Sel.Name {
				case "Fprint", "Fprintf", "Fprintln":
				default:
					return true
				}
				if len(call.Args) == 0 {
					return true
				}
				sinkExpr, ok := resolvedWriterSink(call.Args[0], dir, bindings, fields)
				if !ok {
					return true
				}
				site := callSite{fset: fset, path: pf.Path, call: call, pattern: "fmt." + sel.Sel.Name + "(" + sinkExpr + ", ...)"}
				findings = append(findings, site.finding(RungTaskDefine,
					"writing an injected io.Writer sink bypasses evo's live region the same way os.Stdout would — route it through evo.Init(Config{Stdout: ...}) and evo.Println/Task instead.",
					CertaintyNeedsReview,
				))
				return true
			})
			return true
		})
	}
	return findings
}

// resolvedWriterSink reports whether arg is a bare identifier declared
// directly as io.Writer, or a struct-field access whose field is declared
// io.Writer — resolution is by declared type via bindings/fields, never by
// the identifier's or field's name.
func resolvedWriterSink(arg ast.Expr, dir string, bindings map[string]identBinding, fields writerFieldIndex) (string, bool) {
	switch e := arg.(type) {
	case *ast.Ident:
		if binding, ok := bindings[e.Name]; ok && binding.writerDirect {
			return e.Name, true
		}
		return "", false
	case *ast.SelectorExpr:
		base, ok := e.X.(*ast.Ident)
		if !ok {
			return "", false
		}
		binding, ok := bindings[base.Name]
		if !ok || binding.typeName == "" {
			return "", false
		}
		if fields[dir+"."+binding.typeName][e.Sel.Name] {
			return base.Name + "." + e.Sel.Name, true
		}
		return "", false
	default:
		return "", false
	}
}
