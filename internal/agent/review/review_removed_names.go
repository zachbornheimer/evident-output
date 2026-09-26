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
	"runtime"
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
// subtree actually under review instead of the whole module. recursive is
// false for a single-file review (GoFileAt): dir is then that file's own
// directory, and findingsForFile discards everything outside the one file
// anyway, so there is no need to also type-check every package below it.
func loadPatternFor(root, dir string, recursive bool) (string, error) {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return "", err
	}
	if rel == "." {
		if recursive {
			return "./...", nil
		}
		return ".", nil
	}
	pattern := "./" + filepath.ToSlash(rel)
	if recursive {
		return pattern + "/...", nil
	}
	return pattern, nil
}

// removedNameFindings type-checks the Go module owning dir and runs
// fix.RemovedNameAnalyzers over the packages under dir, returning one
// Finding per diagnostic. recursive selects dir's whole subtree
// (GoDirectoryAt) versus dir's own package only (GoFileAt, which filters
// to one file afterward anyway). overlay substitutes in-memory content for
// on-disk files by absolute path (nil for none) — see
// fix.LoadWithOverlay — so a caller re-reviewing edited source sees its
// own edits reflected in these findings rather than the stale file still
// on disk. found is false only when dir sits outside any Go module (no
// go.mod above it at all) — the ordinary case for a non-Go-module
// directory, where these rule IDs simply don't apply and silence is
// correct. found is true with partial set when a module root was found
// but analysis could not complete (unresolved module graph, missing `go
// mod tidy`, or any other load failure): that is exactly the "could not
// complete" condition GoDirectoryAt reports as Result.Partial, not
// silence and not an error.
func removedNameFindings(dir string, recursive bool, overlay map[string][]byte) (findings []Finding, found, partial bool) {
	// moduleRootAbove always returns an absolute root; dir must match so
	// loadPatternFor's filepath.Rel(root, dir) can resolve it — a caller
	// passing a relative dir (the CLI's `review <dir>` command, e.g. run
	// from a repo root) would otherwise fail Rel with both arguments in
	// different forms and silently degrade to Result.Partial=true.
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, true, true
	}
	dir = abs
	root, ok := moduleRootAbove(dir)
	if !ok {
		return nil, false, false
	}
	pattern, err := loadPatternFor(root, dir, recursive)
	if err != nil {
		return nil, true, true
	}
	pkgs, err := fix.LoadWithOverlay(root, overlay, pattern)
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

// selfModuleRoot is this checkout's own module root, found by walking up
// from this source file's own compile-time recorded path (runtime.Caller,
// the same trick the test suite uses via os.Getwd + filepath.Dir chains —
// see writeRemovedNameModule). kind=package's files map (MCP-017) has no
// filesystem location of its own: its content is decoded inline or read
// from an absolute path and handed over as plain strings, keyed by
// filename only. Every such package that imports evo names this exact
// module, so scratch-loading it as a real package inside this module's own
// tree (moduleRootAbove would otherwise find no go.mod at all) resolves
// that import for real, the same way GoFileAt/GoDirectoryAt resolve a
// consumer's own module. ok is false only when this binary was built
// -trimpath (no source paths survive) or the source tree has moved since
// compilation — removedNamePackageFindings reports Partial in that case
// rather than guessing.
func selfModuleRoot() (root string, ok bool) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		return "", false
	}
	return moduleRootAbove(filepath.Dir(self))
}

// removedNamePackageFindings is GoPackageAt's removedNameFindings: files
// (name -> source, MCP-017's decoded package map) has no shared disk
// location to type-check in place, so it is scratch-written into a real,
// throwaway directory inside this module's own tree (selfModuleRoot) and
// loaded from there — every reviewed package names this same module when
// it imports evo, so this always resolves the same evo declarations
// GoFileAt/GoDirectoryAt would for a real consumer file. found is false
// only when selfModuleRoot can't be resolved (trimpath build); partial
// mirrors removedNameFindings' "module found but analysis didn't
// complete" meaning. Findings come back keyed by the original map's
// filenames, not the scratch directory's absolute paths.
func removedNamePackageFindings(files map[string]string) (findings []Finding, found, partial bool) {
	root, ok := selfModuleRoot()
	if !ok {
		return nil, false, false
	}
	scratch, err := os.MkdirTemp(root, ".evo-review-scratch-")
	if err != nil {
		return nil, true, true
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	overlay := make(map[string][]byte, len(files))
	for name, src := range files {
		overlay[filepath.Join(scratch, filepath.Base(name))] = []byte(src)
	}
	// hasModuleLoadError is not applied here: a pasted snippet reviewed in
	// isolation (kind=package) very often imports something this module's
	// own go.sum has no reason to carry (a demo yaml import, an embedded
	// third-party type) that has nothing to do with evo. go/packages still
	// hands back full type info for the file's own declarations and for
	// every selector it could resolve, including every evo.* receiver
	// call RemovedNameAnalyzers looks at — the same "type errors are
	// expected input" contract LoadWithOverlay documents. Failing the
	// whole package over an unrelated unresolved import would report a
	// false partial for the common case, not the true "evo itself failed
	// to resolve" case this really needs to catch.
	pkgs, err := fix.LoadWithOverlay(scratch, overlay, ".")
	if err != nil || len(pkgs) == 0 {
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
				File:       filepath.Base(d.Filename),
				Line:       d.Line,
				Column:     d.Column,
				Suggestion: removedNameRemediation(d.RuleID),
			})
		}
	}
	return findings, true, false
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
