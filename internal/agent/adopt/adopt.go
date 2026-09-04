// Package adopt inventories non-evo CLI output in an existing codebase and
// proposes a migration plan keyed to the adoption ladder (Init/Main →
// Task/Done → effects → containers → facts/warnings → confirm/dry-run).
// Detection is static and AST-based — every finding is a call site or
// import the compiler itself would resolve the same way, never a guess
// about intent; ambiguous cases are marked NeedsReview instead of silently
// picked one way.
package adopt

import (
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Inventory walks dir for Go source and returns the migration plan.
// It skips vendor/, testdata/, dotfile directories, and generated files it
// can detect via a "Code generated ... DO NOT EDIT" header, matching go's
// own convention.
func Inventory(dir string) (Plan, error) {
	plan := Plan{Directory: dir}
	fset := token.NewFileSet()

	walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if d.IsDir() {
			return skipUninventoried(d)
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		findings, invErr := inventoryPath(fset, path)
		if invErr != nil {
			return fmt.Errorf("inventory %s: %w", path, invErr)
		}
		plan.Findings = append(plan.Findings, findings...)
		return nil
	})
	if walkErr != nil {
		return Plan{}, fmt.Errorf("inventory %s: %w", dir, walkErr)
	}

	sortFindings(plan.Findings)
	plan.RungsTouched = rungsTouched(plan.Findings)
	return plan, nil
}

func skipUninventoried(d os.DirEntry) error {
	skip := d.Name() == "vendor" || d.Name() == "testdata" ||
		(d.Name() != "." && strings.HasPrefix(d.Name(), "."))
	if skip {
		return filepath.SkipDir
	}
	return nil
}

func inventoryPath(fset *token.FileSet, path string) ([]Finding, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if isGenerated(src) {
		return nil, nil
	}
	findings, err := inventoryFile(fset, path, src)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return findings, nil
}

func isGenerated(src []byte) bool {
	head := string(src[:min(len(src), 4096)])
	return strings.Contains(head, "Code generated ") && strings.Contains(head, "DO NOT EDIT")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
}

func rungsTouched(findings []Finding) []Rung {
	order := []Rung{RungInitMain, RungTaskDone, RungEffects, RungFactsWarnings, RungConfirm}
	present := map[Rung]bool{}
	for _, f := range findings {
		present[f.Rung] = true
	}
	var out []Rung
	for _, r := range order {
		if present[r] {
			out = append(out, r)
		}
	}
	return out
}
