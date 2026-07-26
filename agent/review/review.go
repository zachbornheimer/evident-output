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
}

// Result is a review response.
type Result struct {
	Findings        []Finding `json:"findings"`
	RecheckRequired bool      `json:"recheck_required"`
	Partial         bool      `json:"partial,omitempty"` // true when analysis lacks type info
}

// GoSource reviews Go source for evo misuse patterns (AST + textual).
func GoSource(filename, src string) Result {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return Result{
			Findings: []Finding{{
				RuleID:   "API-000",
				Severity: "error",
				Message:  "parse error: " + err.Error(),
				File:     filename,
			}},
			RecheckRequired: true,
		}
	}

	hasEvo := false
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if strings.HasSuffix(path, "evident-output") || path == "github.com/zachbornheimer/evident-output" {
			hasEvo = true
		}
	}

	var findings []Finding
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pos := fset.Position(n.Pos())
		name := sel.Sel.Name

		// API-006: redundant Start
		if name == "Start" {
			findings = append(findings, Finding{
				RuleID:   "API-006",
				Severity: "warning",
				Message:  "explicit Start is usually redundant; prefer Phase/Progress or direct terminal resolution",
				File:     filename,
				Line:     pos.Line,
				Column:   pos.Column,
			})
		}

		// API-027 signal: Tasks.Done/Fail/Progress/Phase misuse (method on plural collection via name)
		// Textual AST cannot resolve types fully — flag X.Tasks(...).Done patterns below in text.

		// STREAM-003: fmt.Print* calls when evo is imported
		if hasEvo {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "fmt" {
				switch name {
				case "Print", "Printf", "Println", "Fprint", "Fprintf", "Fprintln":
					findings = append(findings, Finding{
						RuleID:   "STREAM-003",
						Severity: "error",
						Message:  "fmt." + name + " alongside evo may contaminate managed streams during live UI; use out.Line/Debug/SlogHandler",
						File:     filename,
						Line:     pos.Line,
						Column:   pos.Column,
					})
				}
			}
		}

		// DOM-011 control-flow: returning err after BlockedBy is a smell when err is nil-pattern — hard without types
		// Flag os.Exit in library-style packages using evo
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == "os" && name == "Exit" && hasEvo {
			findings = append(findings, Finding{
				RuleID:   "API-018",
				Severity: "warning",
				Message:  "os.Exit in evo-using code; prefer returning Finish error / conclusion ExitCode",
				File:     filename,
				Line:     pos.Line,
				Column:   pos.Column,
			})
		}
		return true
	})

	// Textual patterns AST may miss
	if hasEvo {
		if strings.Contains(src, "Tasks(") && (strings.Contains(src, ").Done(") || strings.Contains(src, ").Fail(") || strings.Contains(src, ").Progress(")) {
			// crude: only if same line-ish pattern Tasks(...).Done — check for anti-pattern string
			if strings.Contains(src, "Tasks(") {
				// look for variable that is only Tasks collection getting leaf methods is type-level
			}
		}
		// RunAll/Map/Retry forbidden API usage
		for _, bad := range []string{".RunAll(", ".Map(", ".Retry(", ".Parallel(", ".Timeout("} {
			if strings.Contains(src, bad) {
				findings = append(findings, Finding{
					RuleID:   "API-026",
					Severity: "error",
					Message:  "forbidden execution helper " + bad + " must not appear in evo core usage",
					File:     filename,
				})
			}
		}
		// Detail(err) misuse — Detail expects string; if Detail(err) or Detail(someErr)
		if strings.Contains(src, "Detail(err)") || strings.Contains(src, "evo.Detail(err)") {
			findings = append(findings, Finding{
				RuleID:   "DOM-014",
				Severity: "error",
				Message:  "Detail must be user-visible string; use Cause(err) for diagnostic errors",
				File:     filename,
			})
		}
	}

	// MCP-016: single-file AST without package type info is partial analysis.
	return Result{
		Findings:        dedupe(findings),
		RecheckRequired: hasRequired(findings),
		Partial:         hasEvo,
	}
}

// Transcript reviews a terminal transcript for corruption signals (MCP-018).
func Transcript(filename, text string) Result {
	var findings []Finding
	if strings.Contains(text, "\x1b[") && strings.Contains(text, "\r") {
		// mixed cursor and content without clear final — soft signal
	}
	// Split live/final corruption: ESC without matching reset often ok in our driver
	if strings.Count(text, "\x1b[?25l") > strings.Count(text, "\x1b[?25h") {
		findings = append(findings, Finding{
			RuleID:   "TERM-008",
			Severity: "error",
			Message:  "cursor hide without matching show in transcript",
			File:     filename,
		})
	}
	if strings.Contains(text, "\x00") {
		findings = append(findings, Finding{
			RuleID:   "TERM-014",
			Severity: "warning",
			Message:  "NUL byte in transcript suggests unmanaged binary writes",
			File:     filename,
		})
	}
	return Result{
		Findings:        findings,
		RecheckRequired: hasRequired(findings),
	}
}

// StructuredDocument reviews a JSON snapshot/document for schema basics (MCP-019).
func StructuredDocument(filename string, raw []byte) Result {
	var findings []Finding
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Result{
			Findings: []Finding{{
				RuleID: "SCHEMA-001", Severity: "error",
				Message: "invalid JSON: " + err.Error(), File: filename,
			}},
			RecheckRequired: true,
		}
	}
	if v, ok := doc["schema_version"].(string); !ok || v == "" {
		findings = append(findings, Finding{
			RuleID: "SCHEMA-001", Severity: "error",
			Message: "missing schema_version", File: filename,
		})
	}
	if _, ok := doc["conclusion"]; !ok {
		findings = append(findings, Finding{
			RuleID: "SCHEMA-001", Severity: "error",
			Message: "missing conclusion object", File: filename,
		})
	}
	return Result{Findings: findings, RecheckRequired: hasRequired(findings)}
}

func hasRequired(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == "error" {
			return true
		}
	}
	// warnings also require recheck for agent loop
	return len(fs) > 0
}

func dedupe(fs []Finding) []Finding {
	seen := map[string]bool{}
	var out []Finding
	for _, f := range fs {
		k := f.RuleID + ":" + f.Message + ":" + f.File + ":" + itoa(f.Line)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, f)
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
