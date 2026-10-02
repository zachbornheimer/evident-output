package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	idPattern   = regexp.MustCompile(`^C\d+-\d{3}$`)
	skipPattern = regexp.MustCompile(`t\.(Skip|Skipf|SkipNow)\(`)
	spaceRuns   = regexp.MustCompile(`\s+`)
)

func collapseSpace(s string) string {
	return strings.TrimSpace(spaceRuns.ReplaceAllString(s, " "))
}

// lintRegistry returns one line per violation; empty means clean.
func lintRegistry(fsys FileSystem, root string) ([]string, error) {
	entries, err := loadRegistry(fsys, root)
	if err != nil {
		return nil, err
	}
	contract, err := fsys.ReadFile(filepath.Join(root, contractPath))
	if err != nil {
		return nil, fmt.Errorf("lint needs the contract: %w", err)
	}
	normalized := collapseSpace(string(contract))
	var out []string
	seen := map[string]bool{}
	for _, e := range entries {
		out = append(out, lintEntry(e, normalized, seen)...)
	}
	skips, err := lintSkips(fsys, root)
	if err != nil {
		return nil, err
	}
	return append(out, skips...), nil
}

func lintEntry(e Entry, contract string, seen map[string]bool) []string {
	var out []string
	bad := func(format string, args ...any) {
		out = append(out, fmt.Sprintf("%s: %s", e.ID, fmt.Sprintf(format, args...)))
	}
	if !idPattern.MatchString(e.ID) {
		bad("id does not match %s", idPattern)
	}
	if seen[e.ID] {
		bad("duplicate id")
	}
	seen[e.ID] = true
	if e.Quote == "" || !strings.Contains(contract, collapseSpace(e.Quote)) {
		bad("quote does not occur verbatim in %s", contractPath)
	}
	if e.Tier != Tier11 && e.Tier != Tier12 {
		bad("tier %q is not %s or %s", e.Tier, Tier11, Tier12)
	}
	if len(e.Tests) == 0 && strings.TrimSpace(e.Waiver) == "" {
		bad("needs at least one test or a waiver")
	}
	for i, t := range e.Tests {
		if t.Pkg == "" || t.Run == "" {
			bad("test entry %d needs both pkg and run", i)
		}
	}
	return out
}

func lintSkips(fsys FileSystem, root string) ([]string, error) {
	files, err := fsys.WalkFiles(filepath.Join(root, contractTests), ".go")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, file := range files {
		data, err := fsys.ReadFile(file)
		if err != nil {
			return nil, err
		}
		if skipPattern.Match(data) {
			out = append(out, fmt.Sprintf("%s: t.Skip is banned in contract tests", file))
		}
	}
	return out, nil
}
