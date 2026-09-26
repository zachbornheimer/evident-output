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
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/zachbornheimer/evident-output/internal/agent/fix"
	"github.com/zachbornheimer/evident-output/internal/agent/rules"
)

// moduleRootAbove walks up from dir looking for the go.mod that owns it,
// the same way `go build`/`go vet` resolve a module root from any package
// inside it. A directory being reviewed is very often a subpackage
// (internal/app, not the repo root), so a bare os.Stat(dir/go.mod) missed
// every module whose root sits above dir.
func moduleRootAbove(dir string) (root string, ok bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for cur := abs; ; {
		if _, err := os.Stat(filepath.Join(cur, "go.mod")); err == nil {
			return cur, true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", false
		}
		cur = parent
	}
}

// loadPatternFor turns dir into a `go list`-style pattern rooted at root
// (dir itself when they're equal), so fix.Load only type-checks the
// subtree actually under review instead of the whole module.
func loadPatternFor(root, dir string) (string, error) {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return "./...", nil
	}
	return "./" + filepath.ToSlash(rel) + "/...", nil
}

// removedNameFindings type-checks the Go module owning dir and runs
// fix.RemovedNameAnalyzers over the packages under dir, returning one
// Finding per diagnostic. found is false only when dir sits outside any
// Go module (no go.mod above it at all) — the ordinary case for a
// non-Go-module directory, where these rule IDs simply don't apply and
// silence is correct. found is true with partial set when a module root
// was found but analysis could not complete (unresolved module graph,
// missing `go mod tidy`, or any other load failure): that is exactly the
// "could not complete" condition GoDirectoryAt reports as Result.Partial,
// not silence and not an error.
func removedNameFindings(dir string) (findings []Finding, found, partial bool) {
	root, ok := moduleRootAbove(dir)
	if !ok {
		return nil, false, false
	}
	pattern, err := loadPatternFor(root, dir)
	if err != nil {
		return nil, true, true
	}
	pkgs, err := fix.Load(root, pattern)
	if err != nil || len(pkgs) == 0 || hasModuleLoadError(pkgs) {
		return nil, true, true
	}
	results, err := fix.DiagnoseAnalyzers(pkgs, fix.RemovedNameAnalyzers, false)
	if err != nil {
		return nil, true, true
	}
	for _, r := range results {
		for _, d := range r.Diagnostics {
			findings = append(findings, Finding{
				RuleID:     d.RuleID,
				Message:    d.Message,
				File:       d.Filename,
				Line:       d.Line,
				Column:     d.Column,
				Suggestion: removedNameRemediation(d.RuleID),
			})
		}
	}
	return findings, true, false
}

// hasModuleLoadError reports whether packages.Load could not resolve the
// module graph itself (missing go.sum entry, unresolved replace, no
// required module provides package — the `go mod tidy` case the review-gap
// report calls out). This is deliberately narrower than "any pkg.Errors":
// a mid-migration consumer's own type errors from calling a name removed
// in 1.1 are expected input for RemovedNameAnalyzers, not a load failure.
func hasModuleLoadError(pkgs []*packages.Package) bool {
	for _, p := range pkgs {
		for _, e := range p.Errors {
			// ListError is `go list` itself failing (bad pattern, module
			// graph errors reported before any package loads). A "could
			// not import" TypeError is the go/packages driver's own way of
			// reporting a dependency it could not resolve at all (missing
			// go.sum entry, broken replace) — indistinguishable in Kind
			// from a real type error in the reviewed code (e.g. calling a
			// removed method), which is expected input, not a load
			// failure, so only this specific message counts.
			if e.Kind == packages.ListError || strings.Contains(e.Msg, "could not import") {
				return true
			}
		}
	}
	return false
}

// removedNameRemediation is the rule catalog's general fix for ruleID
// (API-070/090/091/120's Remediation is already call-site-independent —
// e.g. "Replace Kept(reason) with Skipped(reason)" — so there is no
// cheaper per-diagnostic suggestion to derive).
func removedNameRemediation(ruleID string) string {
	if r, ok := rules.Explain(ruleID); ok {
		return r.Remediation
	}
	return ""
}
