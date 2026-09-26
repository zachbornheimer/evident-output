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
	// source of truth for these rule IDs — drop any that slipped in from a
	// per-file detector before merging its type-checked results, so a
	// removed name is never reported twice under two different findings.
	all = filterRuleIDs(all, removedNameRuleIDs)
	if removed, ok := removedNameFindings(dir); ok {
		all = append(all, admitDialect(removed, ver)...)
	}
	return dialect.Stamp(newResult(all)), nil
}

// filterRuleIDs drops every finding whose RuleID is in drop.
func filterRuleIDs(fs []Finding, drop map[string]bool) []Finding {
	out := fs[:0]
	for _, f := range fs {
		if drop[f.RuleID] {
			continue
		}
		out = append(out, f)
	}
	return out
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
