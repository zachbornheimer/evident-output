package review

import "go/ast"

// twoZeroFacadeField maps an Option constructor onto the Facades field that
// replaced it.
var twoZeroFacadeField = map[string]string{
	"To": "Stdout", "Diagnostics": "Stderr", "Stdin": "Stdin", "Terminal": "Terminal",
	"Clock": "Clock", "Redact": "Redactor", "Runner": "Process",
}

// twoZeroDroppedOption is what 2.0 offers in place of an Option that has no
// Config field, per docs/migration/2.0.md.
var twoZeroDroppedOption = map[string]string{
	"Width":              "nothing: it has no 2.0 equivalent, Evo picks the presentation from Output and Terminal",
	"Glyphs":             "nothing: it has no 2.0 equivalent, Evo picks the presentation from Output and Terminal",
	"VisibilityDelay":    "nothing: it has no 2.0 equivalent, Evo picks the presentation from Output and Terminal",
	"MaxFrameRate":       "nothing: it has no 2.0 equivalent, the bounds are fixed",
	"MaxEntities":        "nothing: it has no 2.0 equivalent, the bounds are fixed",
	"MaxEvents":          "nothing: it has no 2.0 equivalent, the bounds are fixed",
	"Strict":             "nothing: it has no 2.0 equivalent, misuse is always recorded in Result with a remedy",
	"NoColor":            "the NO_COLOR environment variable",
	"ResultStream":       "Run.ResultWriter(), which keeps the data payload on stdout",
	"DataProjection":     "Config.Output and Run.ResultWriter(), which keeps the data payload on stdout",
	"ExternalProjection": "Config.Output and Run.ResultWriter(), which keeps the data payload on stdout",
	"AlsoWrite":          "io.MultiWriter(os.Stdout, w) assigned to Config.Facades.Stdout",
	"DebugLevel":         "Config.Debug (a slog.Level) or EVO_DEBUG",
	"DebugAddSource":     "Config.Debug (a slog.Level) or EVO_DEBUG",
	"DebugHistory":       "Config.Debug (a slog.Level) or EVO_DEBUG",
	"DebugPane":          "Config.Debug (a slog.Level) or EVO_DEBUG",
}

// twoZeroOptionField is the 2.0 Config field assignment an Option stands
// for; ok is false when 2.0 has no one-to-one field for it.
func (d *recSurfaceDetector) twoZeroOptionField(name string, args []ast.Expr) (string, bool) {
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

// reportDroppedOption reports an Option 2.0 removed with no Config field.
func (d *recSurfaceDetector) reportDroppedOption(call *ast.CallExpr, name, old string) {
	instead, known := twoZeroDroppedOption[name]
	if !known {
		return
	}
	d.report(call, "evo."+name+" was removed in 2.0", "replace "+old+" with "+instead)
	d.cover(call)
}
