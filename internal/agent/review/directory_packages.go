package review

import (
	"cmp"
	"go/parser"
	"go/token"
	"maps"
	"path/filepath"
	"slices"
)

// packageSources groups a directory review's files by Go package, for
// the rules that must see a whole package at once: API-063 follows a Run
// callback into functions declared in other files of the same package.
type packageSources struct {
	fset     *token.FileSet
	packages map[packageKey][]goSource
}

// packageKey is one package: its directory and package clause (a
// directory's external _test package is separate).
type packageKey struct {
	dir  string
	name string
}

// add parses src (already reviewed file by file) into its package. A file
// that does not parse was already reported by GoSourceAt; it is skipped.
func (s *packageSources) add(path string, src []byte) {
	if s.fset == nil {
		s.fset = token.NewFileSet()
		s.packages = map[packageKey][]goSource{}
	}
	file, err := parser.ParseFile(s.fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		return
	}
	key := packageKey{dir: filepath.Dir(path), name: file.Name.Name}
	s.packages[key] = append(s.packages[key], newGoSource(path, file))
}

// findings runs the whole-package rules for desiredVersion, admitting only
// the rules that pin can act on. Findings a file-by-file pass already
// produced come back identical and are removed by dedupe.
func (s *packageSources) findings(desiredVersion string) []Finding {
	var findings []Finding
	for _, key := range slices.SortedFunc(maps.Keys(s.packages), comparePackageKeys) {
		findings = append(findings, detectPackageFacadeInIsolatedRuns(s.fset, s.packages[key])...)
	}
	return admitDialect(findings, desiredVersion)
}

// comparePackageKeys orders packages by directory, then name, so a
// directory review reports in the same order every time.
func comparePackageKeys(a, b packageKey) int {
	return cmp.Or(cmp.Compare(a.dir, b.dir), cmp.Compare(a.name, b.name))
}
