package review

import (
	"go/ast"
	"strings"
)

// supersededOptionFuncs are the Option constructors 1.x passed to its
// constructor (removed in 2.0) and Config.Options; 2.0 has Config and
// Facades fields instead.
var supersededOptionFuncs = map[string]bool{
	"To": true, "Plain": true, "NoColor": true, "Stdin": true,
	"DryRun": true, "VisibilityDelay": true, "Diagnostics": true,
	"Title": true, "ResultStream": true, "Terminal": true, "Clock": true,
	"MaxFrameRate": true, "Width": true, "Redact": true, "Runner": true,
	"MaxEntities": true, "MaxEvents": true, "Strict": true,
	"AlsoWrite": true, "Glyphs": true, "DataProjection": true,
	"ExternalProjection": true, "DebugLevel": true, "DebugAddSource": true,
	"DebugHistory": true, "DebugPane": true,
}

// twoZeroFacadeField maps an Option constructor onto the Facades field that
// replaced it.
var twoZeroFacadeField = map[string]string{
	"To": "Stdout", "Diagnostics": "Stderr", "Stdin": "Stdin", "Terminal": "Terminal",
	"Clock": "Clock", "Redact": "Redactor", "Runner": "Process",
}

const (
	noEquivalentPresentation = "nothing: it has no 2.0 equivalent, Evo picks the presentation from Output and Terminal"
	noEquivalentBounds       = "nothing: it has no 2.0 equivalent, the bounds are fixed"
	debugEquivalent          = "Config.Debug (a slog.Level) or EVO_DEBUG"
	resultWriterEquivalent   = "Config.Output and Run.ResultWriter(), which keeps the data payload on stdout"
)

// twoZeroDroppedOption is what 2.0 offers in place of an Option that has no
// Config field, per docs/migration/2.0.md.
var twoZeroDroppedOption = map[string]string{
	"Width":              noEquivalentPresentation,
	"Glyphs":             noEquivalentPresentation,
	"VisibilityDelay":    noEquivalentPresentation,
	"MaxFrameRate":       noEquivalentBounds,
	"MaxEntities":        noEquivalentBounds,
	"MaxEvents":          noEquivalentBounds,
	"Strict":             "nothing: it has no 2.0 equivalent, misuse is always recorded in Result with a remedy",
	"NoColor":            "the NO_COLOR environment variable",
	"ResultStream":       "Run.ResultWriter(), which keeps the data payload on stdout",
	"DataProjection":     resultWriterEquivalent,
	"ExternalProjection": resultWriterEquivalent,
	"AlsoWrite":          "io.MultiWriter(os.Stdout, w) assigned to Config.Facades.Stdout",
	"DebugLevel":         debugEquivalent,
	"DebugAddSource":     debugEquivalent,
	"DebugHistory":       debugEquivalent,
	"DebugPane":          debugEquivalent,
}

// optionRemovedIn is the release that removed an Option constructor.
func optionRemovedIn(name string) string {
	if release := removedIn(unknownReceiver, name); release != "2.0" {
		return string(release)
	}
	return "1.1"
}

func (d *removedTwoZeroDetector) inspectOptionCall(call *ast.CallExpr, name string) {
	removed := "evo." + name + " was removed in " + optionRemovedIn(name)
	if field, ok := d.optionField(call); ok {
		d.report(call, removed+"; use the Config field", "replace "+d.nodeSrc(call)+" with "+field)
		d.cover(call)
		return
	}
	if instead, known := twoZeroDroppedOption[name]; known {
		d.report(call, removed, "replace "+d.nodeSrc(call)+" with "+instead)
		d.cover(call)
	}
}

// optionField is the 2.0 Config field assignment an Option stands for; ok is
// false when 2.0 has no one-to-one field for it.
func (d *removedTwoZeroDetector) optionField(call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !isEvoIdent(sel.X, d.pkg) {
		return "", false
	}
	name, args := sel.Sel.Name, call.Args
	if len(args) == 0 {
		switch name {
		case "Plain":
			return "Output: " + d.pkg + ".Plain", true
		case "DryRun":
			return "DryRun: true", true
		}
		return "", false
	}
	if len(args) != 1 {
		return "", false
	}
	if name == "Title" {
		return "Name: " + d.nodeSrc(args[0]), true
	}
	if facade, ok := twoZeroFacadeField[name]; ok {
		return "Facades: " + d.pkg + ".Facades{" + facade + ": " + d.nodeSrc(args[0]) + "}", true
	}
	return "", false
}

func (d *removedTwoZeroDetector) inspectComposite(cl *ast.CompositeLit) {
	if d.isCovered(cl) {
		return
	}
	if isEvoConfigLit(cl, d.pkg) {
		d.inspectConfigOptions(cl)
		return
	}
	d.inspectBasis(cl)
	if d.inspectExecSpec(cl) {
		return
	}
	if !isOptionSliceLit(cl, d.pkg) {
		return
	}
	if fields, ok := d.optionSliceFields(cl, nil); ok {
		d.report(cl, "[]evo.Option was removed in 1.1; use Config fields", "replace "+d.nodeSrc(cl)+" with "+fields)
		d.cover(cl)
	}
}

// inspectConfigOptions flags Config.Options. It offers a rewrite only when
// every Option maps one-to-one onto a Config field the literal does not
// already set: a partial rewrite would silently drop the rest, and a repeated
// field does not compile.
func (d *removedTwoZeroDetector) inspectConfigOptions(cl *ast.CompositeLit) {
	d.reportConfigFields(cl)
	set := map[string]bool{}
	for _, elt := range cl.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			set[identName(kv.Key)] = true
		}
	}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if ok && identName(kv.Key) == "Title" {
			d.report(kv, "Config.Title was removed in 2.0; the name is Config.Name", "replace "+d.nodeSrc(kv)+" with Name: "+d.nodeSrc(kv.Value))
			d.cover(kv)
			continue
		}
		if !ok || identName(kv.Key) != "Options" {
			continue
		}
		const message = "Config.Options was removed in 1.1; use Config fields"
		old := d.nodeSrc(kv)
		if slice, isSlice := kv.Value.(*ast.CompositeLit); isSlice && isOptionSliceLit(slice, d.pkg) {
			if fields, ok := d.optionSliceFields(slice, set); ok {
				d.report(kv, message, "replace "+old+" with "+fields)
				d.cover(kv)
				continue
			}
		}
		d.report(kv, message, "move each Option in "+old+" to its Config field by hand; "+
			"at least one has no one-to-one field, or its field is already set, so no automatic rewrite is offered")
		d.cover(kv)
	}
}

// optionSliceFields rewrites an Option slice as Config fields. ok is false
// when any element has no one-to-one field, or maps onto a field in set or
// onto one another element already maps onto.
func (d *removedTwoZeroDetector) optionSliceFields(cl *ast.CompositeLit, set map[string]bool) (string, bool) {
	if len(cl.Elts) == 0 {
		return "", false
	}
	fields := make([]string, 0, len(cl.Elts))
	named := make(map[string]bool, len(cl.Elts))
	for _, elt := range cl.Elts {
		call, ok := elt.(*ast.CallExpr)
		if !ok {
			return "", false
		}
		field, ok := d.optionField(call)
		if !ok {
			return "", false
		}
		key, _, _ := strings.Cut(field, ":")
		if set[key] || named[key] {
			return "", false
		}
		named[key] = true
		fields = append(fields, field)
	}
	return strings.Join(fields, ", "), true
}

func isEvoConfigLit(cl *ast.CompositeLit, pkg string) bool {
	switch t := cl.Type.(type) {
	case *ast.SelectorExpr:
		return isEvoIdent(t.X, pkg) && t.Sel.Name == "Config"
	case *ast.Ident:
		return t.Name == "Config"
	}
	return false
}

func isOptionSliceLit(cl *ast.CompositeLit, pkg string) bool {
	arr, ok := cl.Type.(*ast.ArrayType)
	if !ok {
		return false
	}
	switch t := arr.Elt.(type) {
	case *ast.SelectorExpr:
		return isEvoIdent(t.X, pkg) && t.Sel.Name == "Option"
	case *ast.Ident:
		return t.Name == "Option"
	}
	return false
}
