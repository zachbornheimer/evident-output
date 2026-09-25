package fix

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// optionField maps an exported Option constructor (option_api.go) to the
// Config field assignment it corresponds to, derived by hand against
// internal/engine/construct.go's Config struct. constructor -> "" means
// the constructor was kept only as the Config.Options escape hatch — no
// single field represents it, so it gets a diagnostic naming it instead of
// a guessed fix.
var optionFields = map[string]func(args []string) string{
	"Title":              func(a []string) string { return "Title: " + a[0] },
	"Clock":              func(a []string) string { return "Clock: " + a[0] },
	"DebugAddSource":     func(a []string) string { return "Debug: evo.DebugConfig{AddSource: true}" },
	"DebugLevel":         func(a []string) string { return "Debug: evo.DebugConfig{Level: " + a[0] + "}" },
	"DryRun":             func(a []string) string { return "DryRun: true" },
	"ExternalProjection": func(a []string) string { return "Plain: true" },
	"MaxEntities":        func(a []string) string { return "MaxEntities: " + a[0] },
	"MaxEvents":          func(a []string) string { return "MaxEvents: " + a[0] },
	"MaxFrameRate":       func(a []string) string { return "MaxFrameRate: " + a[0] },
	"NoColor":            func(a []string) string { return "Color: evo.ColorNever" },
	"Plain":              func(a []string) string { return "Plain: true" },
	"Redact":             func(a []string) string { return "Redactor: " + a[0] },
	"ResultStream":       func(a []string) string { return "Result: " + a[0] },
	"Stdin":              func(a []string) string { return "Stdin: " + a[0] },
	"Strict":             func(a []string) string { return "Strict: true" },
	"Terminal":           func(a []string) string { return "Terminal: " + a[0] },
	"To":                 func(a []string) string { return "Stdout: " + a[0] },
	"Width":              func(a []string) string { return "Width: " + a[0] },
}

// noFieldOptions lists Option constructors option_api.go still exports
// that have no single Config field: DataProjection is now a no-op kept
// for source compatibility, and AlsoWrite/Diagnostics/DebugHistory/
// DebugPane/Runner/VisibilityDelay either compose with other state or need
// a value only expressible through Config.Options (the escape hatch), not
// a scalar field assignment.
var noFieldOptions = map[string]bool{
	"AlsoWrite": true, "DataProjection": true, "Diagnostics": true,
	"DebugHistory": true, "DebugPane": true, "Runner": true, "VisibilityDelay": true,
}

// OptionsAnalyzer is API-130: an evo.Init/evo.New call built from
// functional Option arguments, replaced in 1.1 by evo.Config{...}
// construction. Fixed only when every argument maps to a Config field;
// otherwise reported with no fix naming the unmapped constructors, per the
// work order (a call mixing a mappable and an unmappable option is not
// partially rewritten — a half-migrated call reads as done when it isn't).
var OptionsAnalyzer = &analysis.Analyzer{
	Name:     "evooptions",
	Doc:      "flags and fixes evo.Init/New functional-option calls, replaced by Config{...} in 1.1 (API-130)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runOptions,
}

func runOptions(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "Init" && sel.Sel.Name != "New") {
			return
		}
		if !isEvoPackageSelector(pass, sel) {
			return
		}
		// A Config{...} literal call (evo.Init(evo.Config{...})) is
		// already 1.1 shape; only functional-option arguments are ours.
		optCalls, ok := allOptionCalls(pass, call.Args)
		if !ok || len(optCalls) == 0 {
			return
		}
		pass.Report(optionsFinding(pass, call, sel, optCalls))
	})
	return nil, nil
}

type optionCall struct {
	name string
	call *ast.CallExpr
}

// allOptionCalls reports whether every argument is a call to a package-
// level evo Option constructor, returning them in order; a single
// non-constructor argument (a plain value, a Config literal) means this
// evo.Init/New call is not the functional-option shape at all.
func allOptionCalls(pass *analysis.Pass, args []ast.Expr) ([]optionCall, bool) {
	out := make([]optionCall, 0, len(args))
	for _, a := range args {
		call, ok := a.(*ast.CallExpr)
		if !ok {
			return nil, false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !isEvoPackageSelector(pass, sel) {
			return nil, false
		}
		if _, isField := optionFields[sel.Sel.Name]; !isField && !noFieldOptions[sel.Sel.Name] {
			return nil, false
		}
		out = append(out, optionCall{name: sel.Sel.Name, call: call})
	}
	return out, true
}

func optionsFinding(pass *analysis.Pass, call *ast.CallExpr, sel *ast.SelectorExpr, opts []optionCall) analysis.Diagnostic {
	var unmapped []string
	for _, o := range opts {
		if noFieldOptions[o.name] {
			unmapped = append(unmapped, o.name)
		}
	}
	msg := "evo." + sel.Sel.Name + " built from functional Options is legacy: 1.1 constructs evo.Config{...} directly"
	if len(unmapped) > 0 {
		var names strings.Builder
		names.WriteString(unmapped[0])
		for _, u := range unmapped[1:] {
			names.WriteString(", " + u)
		}
		return diag("API-130", call, msg+" — not rewritten: "+names.String()+
			" has no single Config field; keep it via Config.Options or migrate by hand")
	}
	return diag("API-130", call, msg, optionsConfigFix(pass, call, sel, opts))
}

func optionsConfigFix(pass *analysis.Pass, call *ast.CallExpr, sel *ast.SelectorExpr, opts []optionCall) analysis.SuggestedFix {
	alias := evoAlias(pass, sel)
	var fields strings.Builder
	for i, o := range opts {
		if i > 0 {
			fields.WriteString(", ")
		}
		fields.WriteString(optionFields[o.name](argTexts(pass, o.call.Args)))
	}
	newText := sel.Sel.Name + "(" + alias + ".Config{" + fields.String() + "})"
	return analysis.SuggestedFix{
		Message: "replace functional options with a single evo.Config{...}",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: call.End(), NewText: []byte(newText)},
		},
	}
}
