// Package review — removed-name findings (API-070/090/091/120: Warn,
// Step, Kept, ReasonOption/ForSkip/OnTask) are reported by loading the
// reviewed directory as real Go packages and running
// internal/agent/fix's RemovedNameAnalyzers over them, instead of a
// second, review-owned AST walk that can drift from the fixer's typed
// receiver resolution (see fix.WarnAnalyzer's doc comment: recvNamedType
// resolves a call's receiver through go/types back to a declared evo
// type, never by matching an identifier's spelling against a fixed word
// list). The fixer is the single source of truth for what these four
// names mean and how to rewrite them; review only needs to surface its
// diagnostics as Findings.
package review

import (
	"os"
	"path/filepath"

	"github.com/zachbornheimer/evident-output/internal/agent/fix"
)

// removedNameRuleIDs is the exact set DiagnoseAnalyzers(fix.RemovedNameAnalyzers, ...)
// can report, used to keep a directory review's per-file findings from
// ever reporting one of these rule IDs a second time.
var removedNameRuleIDs = map[string]bool{
	"API-070": true, // Warn
	"API-090": true, // Step
	"API-091": true, // Kept
	"API-120": true, // ReasonOption/ForSkip/OnTask
}

// removedNameFindings type-checks dir as a Go module and runs
// fix.RemovedNameAnalyzers over every package it contains, returning one
// Finding per diagnostic. It is best-effort: a directory with no go.mod,
// an unresolved module graph, or any other load failure yields no
// findings rather than failing the surrounding directory review — the
// per-file text/AST detectors already cover everything outside this rule
// set, and a load failure here is exactly the "could not complete"
// condition GoDirectoryAt reports as Partial, not an error.
func removedNameFindings(dir string) ([]Finding, bool) {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return nil, false
	}
	pkgs, err := fix.Load(dir, "./...")
	if err != nil || len(pkgs) == 0 {
		return nil, false
	}
	results, err := fix.DiagnoseAnalyzers(pkgs, fix.RemovedNameAnalyzers, false)
	if err != nil {
		return nil, false
	}
	var findings []Finding
	for _, r := range results {
		for _, d := range r.Diagnostics {
			findings = append(findings, Finding{
				RuleID:  d.RuleID,
				Message: d.Message,
				File:    d.Filename,
				Line:    d.Line,
				Column:  d.Column,
			})
		}
	}
	return findings, true
}
