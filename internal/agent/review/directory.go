package review

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GoDirectory walks dir for Go source and merges per-file GoSource findings.
// Tests are included (review of evo usage in tests is valid). vendor/,
// testdata/, dot-directories, and generated files are skipped.
func GoDirectory(dir string) (Result, error) {
	return GoDirectoryAt(dir, "")
}

// GoDirectoryAt walks dir like GoDirectory, linting as desiredVersion
// (empty uses the go.mod pin; see DialectFor).
func GoDirectoryAt(dir, desiredVersion string) (Result, error) {
	dialect := DialectFor(dir, desiredVersion)
	ver := dialect.Lint()
	var all []Finding
	walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if d.IsDir() {
			return skipUnreviewed(d)
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if generatedHeader(src) {
			return nil
		}
		r := GoSourceAt(path, string(src), ver)
		all = append(all, r.Findings...)
		return nil
	})
	if walkErr != nil {
		return Result{}, fmt.Errorf("review directory %s: %w", dir, walkErr)
	}
	// removedNameFindings (API-070/090/091/120) is the directory's single
	// source of truth for these rule IDs; no per-file detector emits them
	// (see TestNoFileDetectorEmitsRemovedNameRuleIDs), so there is nothing
	// to de-duplicate here.
	removed, found, partial := removedNameFindings(dir)
	if found {
		all = append(all, admitDialect(removed, ver)...)
	}
	res := newResult(all)
	res.Partial = res.Partial || partial
	return dialect.Stamp(res), nil
}

// GoFileAt reviews the single Go file at path on disk: full AST review via
// GoSourceAt, plus API-070/090/091/120 (Warn/Step/Kept/ReasonOption) via
// the same fix.RemovedNameAnalyzers path GoDirectoryAt uses, scoped to
// path's own module and filtered to path so a single-file review reports
// the same removed-name findings a directory review of its parent would.
// Unlike GoSource (source text with no filesystem location), path must be
// a real file — that's what lets it resolve a module root to type-check.
func GoFileAt(path, desiredVersion string) (Result, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", path, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Result{}, fmt.Errorf("resolve %s: %w", path, err)
	}
	dialect := DialectFor(abs, desiredVersion)
	ver := dialect.Lint()
	all := GoSourceAt(abs, string(src), ver).Findings
	removed, found, partial := removedNameFindings(filepath.Dir(abs))
	if found {
		all = append(all, admitDialect(findingsForFile(removed, abs), ver)...)
	}
	res := newResult(all)
	res.Partial = res.Partial || partial
	return dialect.Stamp(res), nil
}

// findingsForFile keeps only the findings whose File matches path, so a
// single-file review scoped to a module subtree doesn't also report
// removed-name findings from sibling files fix.Load had to type-check
// along the way.
func findingsForFile(fs []Finding, path string) []Finding {
	out := make([]Finding, 0, len(fs))
	for _, f := range fs {
		if sameFile(f.File, path) {
			out = append(out, f)
		}
	}
	return out
}

func sameFile(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return a == b
	}
	return absA == absB
}

func skipUnreviewed(d os.DirEntry) error {
	skip := d.Name() == "vendor" || d.Name() == "testdata" ||
		(d.Name() != "." && strings.HasPrefix(d.Name(), "."))
	if skip {
		return filepath.SkipDir
	}
	return nil
}

func generatedHeader(src []byte) bool {
	n := min(len(src), 4096)
	head := string(src[:n])
	return strings.Contains(head, "Code generated ") && strings.Contains(head, "DO NOT EDIT")
}
