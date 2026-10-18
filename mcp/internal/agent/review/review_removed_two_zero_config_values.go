package review

import "go/ast"

// Three v1.0.0 Config fields are not a rename but a change of type: the
// verbosity level became Config.Verbose, the format and projection became
// Config.Output, and Plain became one of Output's values. Each constant of the
// old type maps to one 2.0 field value.

// verbosityFieldValues is the Config.Verbose each Verbosity constant stands
// for; "" is the zero value, so the field is left out.
var verbosityFieldValues = map[string]string{"VerbosityVerbose": "true", "VerbosityNormal": ""}

// outputFieldValues is the Config.Output each Format and Projection constant
// stands for. FormatExternal and ProjectionStreamJSON have no value of their
// own: the data payload is Run.ResultWriter() and the stream is JSONL.
var outputFieldValues = map[string]string{
	"FormatHuman": "Human", "FormatJSON": "JSON", "FormatJSONL": "JSONL", "FormatData": "JSON",
	"ProjectionHuman": "Human", "ProjectionPlain": "Plain", "ProjectionJSON": "JSON",
	"ProjectionJSONL": "JSONL", "ProjectionStreamJSON": "JSONL",
}

// configTypeChange is a Config field whose value 2.0 spells under another
// field.
type configTypeChange struct {
	field    string
	guidance string
}

var configTypeChanges = map[string]configTypeChange{
	"Verbosity":  {"Verbose", "set Verbose: true for the verbose level and leave the field out for the normal one"},
	"Format":     {"Output", "set Output to evo.Human, evo.Plain, evo.JSON or evo.JSONL"},
	"Projection": {"Output", "set Output to evo.Human, evo.Plain, evo.JSON or evo.JSONL"},
	"Plain":      {"Output", "set Output: evo.Plain when Plain was true and leave the field out otherwise"},
}

// configTypeChangeField is the 2.0 field text ("Verbose: true",
// "Output: evo.JSON") a type-changed Config field's value stands for.
// present is false when the value maps to the field's zero value, so the
// field is left out; ok is false when the value is not a constant the guide
// maps.
func (d *removedTwoZeroDetector) configTypeChangeField(key string, value ast.Expr) (field string, present, ok bool) {
	change := configTypeChanges[key]
	constant := d.evoConstantName(value)
	switch key {
	case "Verbosity":
		text, mapped := verbosityFieldValues[constant]
		if !mapped {
			return "", false, false
		}
		return change.field + ": " + text, text != "", true
	case "Plain":
		switch identName(value) {
		case "true":
			return change.field + ": " + d.pkg + ".Plain", true, true
		case "false":
			return "", false, true
		}
		return "", false, false
	}
	text, mapped := outputFieldValues[constant]
	if !mapped {
		return "", false, false
	}
	return change.field + ": " + d.pkg + "." + text, true, true
}

// evoConstantName is the name of the package-level evo identifier e is, or "".
func (d *removedTwoZeroDetector) evoConstantName(e ast.Expr) string {
	if sel, ok := e.(*ast.SelectorExpr); ok && isEvoIdent(sel.X, d.pkg) {
		return sel.Sel.Name
	}
	return ""
}
