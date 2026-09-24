// Package review provides deterministic static review of Evident Output usage.
package review

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
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
		return newResult([]Finding{{
			RuleID:  "API-000",
			Message: "parse error: " + err.Error(),
			File:    filename,
		}})
	}

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

	// GoSource implements its rules fully via AST. Partial is reserved for incomplete
	// typecheck / multi-file analysis — not "evo is imported".
	res := newResult(findings)
	res.DesiredVersion = desiredVersion
	return res
}

// GoPackage reviews multiple Go files in one package with go/types for
// cross-file API resolution without executing package code (MCP-017).
// files maps filename → source. External imports are stubbed so type-check
// stays local to the provided sources.
func GoPackage(files map[string]string) Result {
	if len(files) == 0 {
		return newResult([]Finding{{RuleID: "API-000", Message: "no files provided"}})
	}
	// Package-level evo import (cross-file): STREAM rules apply if any file imports evo.
	pkgHasEvo := false
	for _, src := range files {
		if strings.Contains(src, "evident-output") || strings.Contains(src, `"evo"`) {
			pkgHasEvo = true
			break
		}
	}
	// Per-file textual/AST findings first.
	var all []Finding
	for name, src := range files {
		r := GoSource(name, src)
		all = append(all, r.Findings...)
		// Cross-file STREAM-003: flag fmt.Print* in non-importing files when package uses evo.
		if pkgHasEvo && !strings.Contains(src, "evident-output") {
			if strings.Contains(src, "fmt.Print") || strings.Contains(src, "fmt.Fprint") {
				all = append(all, Finding{
					RuleID:  "STREAM-003",
					Message: "fmt.Print* in package that imports evo may contaminate managed streams (cross-file)",
					File:    name,
				})
			}
		}
	}
	hasEvo := pkgHasEvo

	fset := token.NewFileSet()
	var parsed []*ast.File
	pkgName := "main"
	for name, src := range files {
		f, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
		if err != nil {
			all = append(all, Finding{
				RuleID: "API-000", Message: "parse error in " + name + ": " + err.Error(),
				File: name,
			})
			continue
		}
		pkgName = f.Name.Name
		parsed = append(parsed, f)
	}
	if len(parsed) == 0 {
		res := newResult(all)
		res.Partial = true
		return res
	}

	conf := types.Config{
		// Local-only: missing imports do not abort the whole check.
		Importer: stubImporter{},
		Error:    func(error) {}, // collect via Check return
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Uses:  make(map[*ast.Ident]types.Object),
		Defs:  make(map[*ast.Ident]types.Object),
	}
	_, err := conf.Check(pkgName, fset, parsed, info)
	typed := err == nil || info != nil
	// Cross-file: detect Group/Sequence collection leaf misuse with type info when available.
	for _, f := range parsed {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// If receiver type name is Group/Sequence (package-local), leaf Done/Fail is misuse.
			if tv, ok := info.Types[sel.X]; ok && tv.Type != nil {
				tn := tv.Type.String()
				if (strings.Contains(tn, "GroupHandle") || strings.Contains(tn, "SequenceHandle")) && (sel.Sel.Name == "Done" || sel.Sel.Name == "Fail" || sel.Sel.Name == "Progress") {
					pos := fset.Position(n.Pos())
					all = append(all, Finding{
						RuleID:  "API-027",
						Message: fmt.Sprintf("typed: %s.%s on collection type %s is forbidden", tn, sel.Sel.Name, tn),
						File:    pos.Filename,
						Line:    pos.Line,
						Column:  pos.Column,
					})
				}
			}
			return true
		})
	}
	// Partial only when type check fully failed and we lack multi-file coverage.
	partial := !typed || hasEvo && err != nil
	if len(files) >= 2 && err == nil {
		partial = false // MCP-017: multi-file types resolved
	}
	if err != nil && len(files) >= 2 {
		// Still mark that cross-file parse ran; type errors may be from stubs.
		partial = true
		all = append(all, Finding{
			RuleID:  "MCP-017",
			Message: "cross-file typecheck incomplete: " + err.Error(),
		})
	}
	res := newResult(all)
	res.Partial = partial
	return res
}

// stubImporter satisfies go/types for external imports without loading code.
type stubImporter struct{}

func (stubImporter) Import(path string) (*types.Package, error) {
	// Return an empty package so Check can continue for local symbols.
	return types.NewPackage(path, path[strings.LastIndex(path, "/")+1:]), nil
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
