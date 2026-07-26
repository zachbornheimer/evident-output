// Package review provides deterministic static review of Evident Output usage.
package review

import (
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
}

// GoSource reviews Go source for common evo misuse patterns.
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
		// Detect out.Start(...) redundant patterns is future work.
		// Flag fmt.Print* in same file when evo import present — STREAM-003 signal.
		if id, ok := call.Fun.(*ast.SelectorExpr); ok {
			_ = id
		}
		name := sel.Sel.Name
		if name == "Start" {
			pos := fset.Position(n.Pos())
			findings = append(findings, Finding{
				RuleID:   "API-006",
				Severity: "warning",
				Message:  "explicit Start is usually redundant; prefer Phase/Progress or direct terminal resolution",
				File:     filename,
				Line:     pos.Line,
				Column:   pos.Column,
			})
		}
		return true
	})
	// Textual STREAM checks
	if strings.Contains(src, "fmt.Printf") && strings.Contains(src, "evo.") {
		findings = append(findings, Finding{
			RuleID:   "STREAM-003",
			Severity: "warning",
			Message:  "fmt.Printf alongside evo may contaminate managed streams during live UI",
			File:     filename,
		})
	}
	return Result{
		Findings:        findings,
		RecheckRequired: len(findings) > 0,
	}
}
