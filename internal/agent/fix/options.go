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
// optionFields functions take the resolved evo package alias so the
// generated DebugConfig{} / ColorNever literals qualify with whatever
// name the source imports evo under, instead of hardcoding "evo." — a
// call site that imports evo under an alias (e.g. `import e "…/evo"`)
// must not receive an undefined "evo" reference in its own fix.
var optionFields = map[string]func(alias string, args []string) string{
	"Title":              func(_ string, a []string) string { return "Title: " + a[0] },
	"Clock":              func(_ string, a []string) string { return "Clock: " + a[0] },
	"DebugAddSource":     func(alias string, a []string) string { return "Debug: " + alias + ".DebugConfig{AddSource: true}" },
	"DebugLevel":         func(alias string, a []string) string { return "Debug: " + alias + ".DebugConfig{Level: " + a[0] + "}" },
	"DryRun":             func(_ string, a []string) string { return "DryRun: true" },
	"ExternalProjection": func(_ string, a []string) string { return "Plain: true" },
	"MaxEntities":        func(_ string, a []string) string { return "MaxEntities: " + a[0] },
	"MaxEvents":          func(_ string, a []string) string { return "MaxEvents: " + a[0] },
	"MaxFrameRate":       func(_ string, a []string) string { return "MaxFrameRate: " + a[0] },
	"NoColor":            func(alias string, a []string) string { return "Color: " + alias + ".ColorNever" },
	"Plain":              func(_ string, a []string) string { return "Plain: true" },
	"Redact":             func(_ string, a []string) string { return "Redactor: " + a[0] },
	"ResultStream":       func(_ string, a []string) string { return "Result: " + a[0] },
	"Stdin":              func(_ string, a []string) string { return "Stdin: " + a[0] },
	"Strict":             func(_ string, a []string) string { return "Strict: true" },
	"Terminal":           func(_ string, a []string) string { return "Terminal: " + a[0] },
	"To":                 func(_ string, a []string) string { return "Stdout: " + a[0] },
	"Width":              func(_ string, a []string) string { return "Width: " + a[0] },
	"VisibilityDelay":    func(alias string, a []string) string { return "VisibilityDelay: " + alias + ".Delay(" + a[0] + ")" },
}

// configField names the Config struct field a constructor writes, so two
// constructors writing the same field can be detected before they collide
// in a struct literal. DebugAddSource and DebugLevel both write "Debug",
// but into disjoint DebugConfig sub-fields, so they alone are allowed to
// coexist — mergeDebug below combines them into one literal instead of one
// overwriting the other.
var configField = map[string]string{
	"Title": "Title", "Clock": "Clock", "DebugAddSource": "Debug", "DebugLevel": "Debug",
	"DryRun": "DryRun", "ExternalProjection": "Plain", "MaxEntities": "MaxEntities",
	"MaxEvents": "MaxEvents", "MaxFrameRate": "MaxFrameRate", "NoColor": "Color",
	"Plain": "Plain", "Redact": "Redactor", "ResultStream": "Result", "Stdin": "Stdin",
	"Strict": "Strict", "Terminal": "Terminal", "To": "Stdout", "Width": "Width",
	"VisibilityDelay": "VisibilityDelay",
}

// noFieldOptions lists Option constructors option_api.go still exports
// that have no single Config field: DataProjection is now a no-op kept
// for source compatibility, and AlsoWrite/Diagnostics/DebugHistory/
// DebugPane/Runner either compose with other state or need a value only
// expressible through Config.Options (the escape hatch), not a scalar
// field assignment. VisibilityDelay DOES have a field
// (internal/engine/construct.go Config.VisibilityDelay) — see optionFields.
var noFieldOptions = map[string]bool{
	"AlsoWrite": true, "DataProjection": true, "Diagnostics": true,
	"DebugHistory": true, "DebugPane": true, "Runner": true,
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
	if dup := duplicateField(opts); dup != "" {
		return diag("API-130", call, msg+" — not rewritten: more than one option writes the "+dup+
			" Config field, which would produce a duplicate-field struct literal that does not compile; merge them by hand")
	}
	return diag("API-130", call, msg, optionsConfigFix(pass, call, sel, opts))
}

// duplicateField returns the Config field name written by more than one
// option call, or "" when every option targets a distinct field.
// DebugAddSource and DebugLevel both target "Debug" but are not a
// duplicate — they merge into one DebugConfig literal (mergeDebugFields) —
// so a second occurrence of either of those two specifically is allowed;
// any other repeat, or a genuine second Debug-field constructor beyond
// that pair, is refused rather than silently overwritten.
func duplicateField(opts []optionCall) string {
	seen := map[string]int{}
	debugNames := map[string]int{}
	for _, o := range opts {
		field := configField[o.name]
		if field == "Debug" {
			debugNames[o.name]++
			continue
		}
		seen[field]++
	}
	for field, n := range seen {
		if n > 1 {
			return field
		}
	}
	for name, n := range debugNames {
		if n > 1 {
			return "Debug (" + name + " given more than once)"
		}
	}
	return ""
}

func optionsConfigFix(pass *analysis.Pass, call *ast.CallExpr, sel *ast.SelectorExpr, opts []optionCall) analysis.SuggestedFix {
	alias := evoAlias(pass, sel)
	var fields strings.Builder
	first := true
	writeField := func(text string) {
		if !first {
			fields.WriteString(", ")
		}
		fields.WriteString(text)
		first = false
	}

	var debugSubfields []string
	for _, o := range opts {
		if configField[o.name] == "Debug" {
			sub := optionFields[o.name](alias, argTexts(pass, o.call.Args))
			// optionFields renders the full "Debug: <alias>.DebugConfig{...}"
			// text for a lone Debug option; strip that wrapper here so
			// two Debug options can share one DebugConfig{...} literal
			// instead of one silently overwriting the other's field.
			sub = strings.TrimPrefix(sub, "Debug: "+alias+".DebugConfig{")
			sub = strings.TrimSuffix(sub, "}")
			debugSubfields = append(debugSubfields, sub)
			continue
		}
		writeField(optionFields[o.name](alias, argTexts(pass, o.call.Args)))
	}
	if len(debugSubfields) > 0 {
		writeField("Debug: " + alias + ".DebugConfig{" + strings.Join(debugSubfields, ", ") + "}")
	}

	newText := sel.Sel.Name + "(" + alias + ".Config{" + fields.String() + "})"
	return analysis.SuggestedFix{
		Message: "replace functional options with a single evo.Config{...}",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: call.End(), NewText: []byte(newText)},
		},
	}
}
