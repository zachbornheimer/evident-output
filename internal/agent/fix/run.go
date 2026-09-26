package fix

import (
	"fmt"
	"go/format"
	"os"
	"sort"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"
)

// Diagnostic is one analyzer finding, flattened to the package/file/line
// the CLI reports and whether its fix survived to be applied.
type Diagnostic struct {
	RuleID   string
	Message  string
	Filename string
	Line     int
	Column   int
	Fixed    bool
}

// Result is one loaded package's findings.
type Result struct {
	PkgPath     string
	Diagnostics []Diagnostic
}

// edit is one resolved TextEdit, tagged with the diagnostic it belongs to
// (owner) so two diagnostics whose edits overlap — a call an outer
// analyzer rewrites whole while an inner one edits one of its arguments,
// e.g. KeptAnalyzer's Skipped(reason) rewrite spanning the same bytes
// ReasonOptionAnalyzer deletes evo.ForSkip() from — can be told apart:
// resolveEdits keeps the first owner it sees per file and drops every
// later edit that overlaps it, rather than splicing both into corrupt
// source.
type edit struct {
	owner      int
	start, end int
	newText    []byte
}

// diagBuild accumulates one package's diagnostics plus their raw edits
// (still by *ast* position, not yet resolved to file offsets) while every
// analyzer runs, so resolveEdits can run once per file afterward.
type diagBuild struct {
	diags []Diagnostic
	edits map[string][]edit // filename -> edits, owner = index into diags
}

// Load loads every package matching patterns with full type information,
// the way `go vet`/`go build` would, so every analyzer's typed receiver
// resolution has real go/types data to work against.
func Load(dir string, patterns ...string) ([]*packages.Package, error) {
	return LoadWithOverlay(dir, nil, patterns...)
}

// LoadWithOverlay is Load, substituting overlay's content for the files it
// keys (by absolute path) instead of their on-disk content — the same
// mechanism `gopls` uses to type-check an editor buffer that hasn't been
// saved. A caller reviewing edited source that has a real path (the
// AGENTS.md review/apply/re-review loop) needs its own edits reflected in
// removed-name analysis, not the stale file still on disk. A nil overlay
// behaves exactly like Load.
func LoadWithOverlay(dir string, overlay map[string][]byte, patterns ...string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Dir: dir,
		// Tests: true loads each package's _test.go variant alongside the
		// package proper. Without it, a consumer test file that calls
		// Step/Kept/Warn or an Option constructor gets no diagnostic and
		// silently stops compiling the moment those names are removed.
		Tests:   true,
		Overlay: overlay,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("loading packages %v: %w", patterns, err)
	}
	// Type errors are expected input, not a load failure: the whole point
	// of `fix` is migrating code that no longer compiles against the
	// removed names. packages.Load still hands back syntax and best-effort
	// type info for a package with errors, which is all every analyzer's
	// fallback path (evotype.go, evolocals.go) needs.
	return pkgs, nil
}

// runAnalyzers runs every analyzer in analyzers over one package and
// returns its diagnostics with each one's edits still keyed by owner
// index.
func runAnalyzers(pkg *packages.Package, analyzers []*analysis.Analyzer) (*diagBuild, error) {
	b := &diagBuild{edits: map[string][]edit{}}
	for _, a := range analyzers {
		pass := &analysis.Pass{
			Analyzer:  a,
			Fset:      pkg.Fset,
			Files:     pkg.Syntax,
			Pkg:       pkg.Types,
			TypesInfo: pkg.TypesInfo,
			ResultOf:  map[*analysis.Analyzer]any{},
			Report: func(d analysis.Diagnostic) {
				pos := pkg.Fset.Position(d.Pos)
				owner := len(b.diags)
				b.diags = append(b.diags, Diagnostic{
					RuleID: d.Category, Message: d.Message,
					Filename: pos.Filename, Line: pos.Line, Column: pos.Column,
				})
				if len(d.SuggestedFixes) == 0 {
					return
				}
				for _, te := range d.SuggestedFixes[0].TextEdits {
					s, e := pkg.Fset.Position(te.Pos), pkg.Fset.Position(te.End)
					if s.Filename == "" {
						continue
					}
					b.edits[s.Filename] = append(b.edits[s.Filename], edit{
						owner: owner, start: s.Offset, end: e.Offset, newText: te.NewText,
					})
				}
			},
		}
		for _, req := range a.Requires {
			res, err := req.Run(pass)
			if err != nil {
				return nil, fmt.Errorf("analyzer %s dependency %s: %w", a.Name, req.Name, err)
			}
			pass.ResultOf[req] = res
		}
		if _, err := a.Run(pass); err != nil {
			return nil, fmt.Errorf("analyzer %s on %s: %w", a.Name, pkg.PkgPath, err)
		}
	}
	return b, nil
}

// resolveEdits sorts one file's edits by start offset and keeps the first
// owner's edits, dropping every later edit whose range overlaps one
// already kept. It returns the accepted edits and the set of owner
// indices they belong to.
func resolveEdits(edits []edit) (accepted []edit, owners map[int]bool) {
	// Collapse byte-identical edits (same start, end and replacement
	// text) into one physical edit before the overlap pass. Two
	// diagnostics in the same file that each need the same prerequisite
	// — e.g. two Warn fixes each inserting the same zero-length `import
	// evo "..."` at the package clause — produce identical edits that
	// are not really in conflict; applying every copy would duplicate
	// the import. Every owner in a collapsed group is credited as fixed
	// once the group's single representative edit survives below.
	type key struct {
		start, end int
		text       string
	}
	groupOwners := map[key][]int{}
	var order []key
	repr := map[key]edit{}
	for _, e := range edits {
		k := key{e.start, e.end, string(e.newText)}
		if _, ok := repr[k]; !ok {
			order = append(order, k)
			repr[k] = e
		}
		groupOwners[k] = append(groupOwners[k], e.owner)
	}
	deduped := make([]edit, 0, len(order))
	for _, k := range order {
		deduped = append(deduped, repr[k])
	}

	sorted := deduped
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].start != sorted[j].start {
			return sorted[i].start < sorted[j].start
		}
		return sorted[i].owner < sorted[j].owner
	})
	repOwners := map[int]bool{}
	dropped := map[int]bool{}
	lastEnd := -1
	for _, e := range sorted {
		if dropped[e.owner] {
			continue
		}
		if e.start < lastEnd {
			dropped[e.owner] = true
			delete(repOwners, e.owner)
			continue
		}
		accepted = append(accepted, e)
		repOwners[e.owner] = true
		if e.end > lastEnd {
			lastEnd = e.end
		}
	}
	owners = map[int]bool{}
	for _, k := range order {
		if !repOwners[repr[k].owner] {
			continue
		}
		for _, o := range groupOwners[k] {
			owners[o] = true
		}
	}
	dropped = map[int]bool{}
	for _, k := range order {
		if repOwners[repr[k].owner] {
			continue
		}
		for _, o := range groupOwners[k] {
			dropped[o] = true
		}
	}
	// An owner with any dropped edit must lose all its edits: a fix's
	// TextEdits are one unit (e.g. the rewritten call plus its `fmt`
	// import), and applying half of it is worse than applying none.
	final := accepted[:0]
	for _, e := range accepted {
		if !dropped[e.owner] {
			final = append(final, e)
		}
	}
	for o := range dropped {
		delete(owners, o)
	}
	return final, owners
}

// dedupAcrossVariants drops diagnostics (and their owned edits) already
// seen under the same (file, line, column, rule) key in seenDiag, then
// records the ones it keeps. Owner indices in the returned diagBuild's
// edits are remapped to match the filtered diags slice.
func dedupAcrossVariants(b *diagBuild, seenDiag map[[4]any]bool) *diagBuild {
	remap := make(map[int]int, len(b.diags))
	out := &diagBuild{edits: map[string][]edit{}}
	for i, d := range b.diags {
		key := [4]any{d.Filename, d.Line, d.Column, d.RuleID}
		if seenDiag[key] {
			continue
		}
		seenDiag[key] = true
		remap[i] = len(out.diags)
		out.diags = append(out.diags, d)
	}
	for filename, edits := range b.edits {
		for _, e := range edits {
			newOwner, ok := remap[e.owner]
			if !ok {
				continue
			}
			e.owner = newOwner
			out.edits[filename] = append(out.edits[filename], e)
		}
	}
	return out
}

// Diagnose runs every analyzer in Analyzers over every loaded package and,
// when apply is true, writes each surviving fix's edits back to disk
// (gofmt-formatted).
func Diagnose(pkgs []*packages.Package, apply bool) ([]Result, error) {
	return DiagnoseAnalyzers(pkgs, Analyzers, apply)
}

// DiagnoseAnalyzers is Diagnose over an explicit analyzer subset — the
// caller decides which removed-name families to run instead of always
// running the full registry (internal/agent/review's directory path runs
// only RemovedNameAnalyzers, so its findings never include a rule an
// unrelated review-side detector already reports).
func DiagnoseAnalyzers(pkgs []*packages.Package, analyzers []*analysis.Analyzer, apply bool) ([]Result, error) {
	var results []Result
	writes := map[string][]edit{}
	// Tests: true (Load) makes packages.Load return synthetic variants of
	// each package (pkg, pkg [pkg.test], pkg_test [pkg.test]) that all
	// share the package's non-test files. Without dedup, a diagnostic on
	// a shared file would be reported — and, when apply, its edit queued
	// — once per variant. seenDiag tracks (file, line, col, rule) across
	// every package this call processes.
	seenDiag := map[[4]any]bool{}

	for _, pkg := range pkgs {
		b, err := runAnalyzers(pkg, analyzers)
		if err != nil {
			return nil, err
		}
		b = dedupAcrossVariants(b, seenDiag)
		if len(b.diags) == 0 {
			continue
		}
		fixedOwners := map[int]bool{}
		for filename, edits := range b.edits {
			accepted, owners := resolveEdits(edits)
			for o := range owners {
				fixedOwners[o] = true
			}
			if apply {
				writes[filename] = append(writes[filename], accepted...)
			}
		}
		for i := range b.diags {
			b.diags[i].Fixed = apply && fixedOwners[i]
		}
		sort.Slice(b.diags, func(i, j int) bool {
			if b.diags[i].Filename != b.diags[j].Filename {
				return b.diags[i].Filename < b.diags[j].Filename
			}
			return b.diags[i].Line < b.diags[j].Line
		})
		results = append(results, Result{PkgPath: pkg.PkgPath, Diagnostics: b.diags})
	}

	if apply {
		if err := writeEdits(writes); err != nil {
			return results, err
		}
	}
	return results, nil
}

// Diffs runs Diagnose in dry-run mode and returns each fixable file's
// unified diff, for the `-diff` CLI flag.
func Diffs(pkgs []*packages.Package) (map[string]string, error) {
	return DiffsAnalyzers(pkgs, Analyzers)
}

// DiffsAnalyzers is Diffs over an explicit analyzer subset; see
// DiagnoseAnalyzers.
func DiffsAnalyzers(pkgs []*packages.Package, analyzers []*analysis.Analyzer) (map[string]string, error) {
	byFile := map[string][]edit{}
	seenDiag := map[[4]any]bool{} // see dedupAcrossVariants in Diagnose
	for _, pkg := range pkgs {
		b, err := runAnalyzers(pkg, analyzers)
		if err != nil {
			return nil, err
		}
		b = dedupAcrossVariants(b, seenDiag)
		for filename, edits := range b.edits {
			accepted, _ := resolveEdits(edits)
			byFile[filename] = append(byFile[filename], accepted...)
		}
	}
	diffs := map[string]string{}
	for filename, edits := range byFile {
		if len(edits) == 0 {
			continue
		}
		before, after, err := renderEdits(filename, edits)
		if err != nil {
			return nil, err
		}
		if d := unifiedDiff(filename, string(before), string(after)); d != "" {
			diffs[filename] = d
		}
	}
	return diffs, nil
}

// renderEdits applies edits to filename's on-disk content in memory,
// returning the original and gofmt-formatted result. Edits must already
// be non-overlapping (resolveEdits guarantees this).
func renderEdits(filename string, edits []edit) (before, after []byte, err error) {
	src, err := os.ReadFile(filename)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", filename, err)
	}
	sorted := append([]edit(nil), edits...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].start > sorted[j].start })
	out := src
	for _, e := range sorted {
		if e.start < 0 || e.end > len(out) || e.start > e.end {
			continue
		}
		var buf []byte
		buf = append(buf, out[:e.start]...)
		buf = append(buf, e.newText...)
		buf = append(buf, out[e.end:]...)
		out = buf
	}
	formatted, err := format.Source(out)
	if err != nil {
		return src, nil, fmt.Errorf("gofmt %s after applying fixes: %w", filename, err)
	}
	return src, formatted, nil
}

func writeEdits(fileEdits map[string][]edit) error {
	for filename, edits := range fileEdits {
		if len(edits) == 0 {
			continue
		}
		_, formatted, err := renderEdits(filename, edits)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filename, formatted, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", filename, err)
		}
	}
	return nil
}
