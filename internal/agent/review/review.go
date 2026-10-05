// Package review provides deterministic static review of Evident Output usage.
package review

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// Finding is one review result.
type Finding struct {
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
	// Suggestion is a one-line, mechanically-applicable fix using the
	// matched call site's own identifiers where cheaply derivable from the
	// AST/text match — e.g. "replace fmt.Println(...) with out.Println".
	// It always names the simplest front-door API (Task.Skipped,
	// Writer, Each, Confirm, Init+Main, ...), never Plan/Changes or
	// hand-rolled composition. Empty when no substitution is cheap to
	// derive; the rule's GoodCode remains the fallback teaching example.
	Suggestion string `json:"suggestion,omitempty"`
	// RequiredVersion is the minimum evident-output release the Suggestion's
	// API requires (e.g. "1.0.0" for Verify/evo.File/evo.Exec/Sequence).
	// Empty means the suggestion is valid at any supported version.
	RequiredVersion string `json:"required_version,omitempty"`
}

// Result is a review response.
type Result struct {
	Findings        []Finding `json:"findings"`
	RecheckRequired bool      `json:"recheck_required"`
	// Partial is true only when analysis could not complete (parse failure,
	// empty package, typecheck incomplete). Complete GoSource AST review is
	// never partial merely because evo is imported — partial+recheck=false
	// confuses agents into ignoring a shippable result.
	Partial bool `json:"partial,omitempty"`
	// DesiredVersion is the pin the caller asked review to compare against
	// (MCP desired_version, or the directory go.mod pin when omitted).
	DesiredVersion string `json:"desired_version,omitempty"`
	// ModuleVersion is the evident-output require in the reviewed go.mod.
	ModuleVersion string `json:"module_version,omitempty"`
	// ReplacePath is a filesystem replace for evident-output, if any.
	ReplacePath string `json:"replace_path,omitempty"`
}

// GoSource reviews Go source for evo misuse patterns (AST + textual)
// against the current rec dialect.
func GoSource(filename, src string) Result {
	return GoSourceAt(filename, src, "")
}

// GoSourceAt reviews src as it would be written for desiredVersion
// (empty means the current rec dialect).
func GoSourceAt(filename, src, desiredVersion string) Result {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return newResult([]Finding{parseErrorFinding(filename, err)})
	}
	// GoSource implements its rules fully via AST. Partial is reserved for incomplete
	// typecheck / multi-file analysis — not "evo is imported".
	res := newResult(reviewFile(filename, src, f, fset, desiredVersion))
	res.DesiredVersion = desiredVersion
	return res
}

// reviewFile runs every file detector that admits parsed file f.
func reviewFile(filename, src string, f *ast.File, fset *token.FileSet, desiredVersion string) []Finding {
	in := fileInput{
		filename:       filename,
		src:            src,
		desiredVersion: desiredVersion,
		file:           f,
		fset:           fset,
		hasEvo:         evoImportName(f) != "",
	}
	var findings []Finding
	for _, d := range fileDetectors {
		if d.admits(in) {
			findings = append(findings, admitDialect(d.run(in), desiredVersion)...)
		}
	}
	return findings
}

func parseErrorFinding(filename string, err error) Finding {
	return Finding{RuleID: "API-000", Message: "parse error: " + err.Error(), File: filename}
}

// Transcript reviews a terminal transcript for corruption signals (MCP-018).
func Transcript(filename, text string) Result {
	var findings []Finding
	// Split live/final corruption: ESC without matching reset often ok in our driver
	if strings.Count(text, "\x1b[?25l") > strings.Count(text, "\x1b[?25h") {
		findings = append(findings, Finding{
			RuleID:  "TERM-008",
			Message: "cursor hide without matching show in transcript",
			File:    filename,
		})
	}
	if strings.Contains(text, "\x00") {
		findings = append(findings, Finding{
			RuleID:  "TERM-014",
			Message: "NUL byte in transcript suggests unmanaged binary writes",
			File:    filename,
		})
	}
	return newResult(findings)
}

// StructuredDocument reviews a JSON snapshot/document for schema basics (MCP-019).
func StructuredDocument(filename string, raw []byte) Result {
	var findings []Finding
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return newResult([]Finding{{RuleID: "SCHEMA-001", Message: "invalid JSON: " + err.Error(), File: filename}})
	}
	if v, ok := doc["schema_version"].(string); !ok || v == "" {
		findings = append(findings, Finding{
			RuleID: "SCHEMA-001", Message: "missing schema_version", File: filename,
		})
	}
	if _, ok := doc["conclusion"]; !ok {
		findings = append(findings, Finding{
			RuleID: "SCHEMA-001", Message: "missing conclusion object", File: filename,
		})
	}
	return newResult(findings)
}
