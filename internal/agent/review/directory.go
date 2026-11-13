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
		r := GoSource(path, string(src))
		all = append(all, r.Findings...)
		return nil
	})
	if walkErr != nil {
		return Result{}, fmt.Errorf("review directory %s: %w", dir, walkErr)
	}
	all = dedupe(all)
	return Result{
		Findings:        all,
		RecheckRequired: hasRequired(all),
	}, nil
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
	n := len(src)
	if n > 4096 {
		n = 4096
	}
	head := string(src[:n])
	return strings.Contains(head, "Code generated ") && strings.Contains(head, "DO NOT EDIT")
}
