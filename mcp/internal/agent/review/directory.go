package review

import (
	"fmt"
	"io/fs"
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
	tree := os.DirFS(dir)
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
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("relate %s to %s: %w", path, dir, err)
		}
		src, err := fs.ReadFile(tree, filepath.ToSlash(rel))
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
	return dialect.Stamp(newResult(all)), nil
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
