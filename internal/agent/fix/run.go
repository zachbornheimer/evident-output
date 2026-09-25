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
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Dir: dir,
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

// runAnalyzers runs every Analyzer over one package and returns its
// diagnostics with each one's edits still keyed by owner index.
func runAnalyzers(pkg *packages.Package) (*diagBuild, error) {
	b := &diagBuild{edits: map[string][]edit{}}
	for _, a := range Analyzers {
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
	sorted := append([]edit(nil), edits...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].start != sorted[j].start {
			return sorted[i].start < sorted[j].start
		}
		return sorted[i].owner < sorted[j].owner
	})
	owners = map[int]bool{}
	dropped := map[int]bool{}
	lastEnd := -1
	for _, e := range sorted {
		if dropped[e.owner] {
			continue
		}
		if e.start < lastEnd {
			dropped[e.owner] = true
			delete(owners, e.owner)
			continue
		}
		accepted = append(accepted, e)
		owners[e.owner] = true
		if e.end > lastEnd {
			lastEnd = e.end
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

// Diagnose runs every analyzer in Analyzers over every loaded package and,
// when apply is true, writes each surviving fix's edits back to disk
// (gofmt-formatted).
func Diagnose(pkgs []*packages.Package, apply bool) ([]Result, error) {
	var results []Result
	writes := map[string][]edit{}

	for _, pkg := range pkgs {
		b, err := runAnalyzers(pkg)
		if err != nil {
			return nil, err
		}
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
	byFile := map[string][]edit{}
	for _, pkg := range pkgs {
		b, err := runAnalyzers(pkg)
		if err != nil {
			return nil, err
		}
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
