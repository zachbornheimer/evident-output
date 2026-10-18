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

	// contractHeading matches a numbered section heading of CONTRACT.md.
	contractHeading = regexp.MustCompile(`(?m)^## (\d+)\. `)
	// sectionCitation matches a waiver's citation of a contract section:
	// "CONTRACT §4" or "CONTRACT.md §4".
	sectionCitation = regexp.MustCompile(`CONTRACT(?:\.md)? §(\d+)\b`)
)

// contractText is CONTRACT.md as the lint reads it: whitespace-collapsed for
// quote matching, and the numbered sections a waiver may cite.
type contractText struct {
	normalized string
	sections   map[string]bool
}

func parseContract(raw string) contractText {
	sections := map[string]bool{}
	for _, m := range contractHeading.FindAllStringSubmatch(raw, -1) {
		sections[m[1]] = true
	}
	return contractText{normalized: collapseSpace(raw), sections: sections}
}

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
	parsed := parseContract(string(contract))
	var out []string
	seen := map[string]bool{}
	for _, e := range entries {
		out = append(out, lintEntry(e, parsed, seen)...)
	}
	skips, err := lintSkips(fsys, root)
	if err != nil {
		return nil, err
	}
	return append(out, skips...), nil
}

func lintEntry(e Entry, contract contractText, seen map[string]bool) []string {
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
	quoteOK := e.Quote != "" && strings.Contains(contract.normalized, collapseSpace(e.Quote))
	if !quoteOK && !e.Waived() {
		bad("quote does not occur verbatim in %s", contractPath)
	}
	if e.Tier != Tier11 && e.Tier != Tier12 {
		bad("tier %q is not %s or %s", e.Tier, Tier11, Tier12)
	}
	if len(e.Tests) == 0 && !e.Waived() {
		bad("needs at least one test or a waiver")
	}
	for i, t := range e.Tests {
		if t.Pkg == "" || t.Run == "" {
			bad("test entry %d needs both pkg and run", i)
		}
	}
	return append(out, lintWaiver(e, contract)...)
}

// lintWaiver checks that a waiver says why. A tier 1.1 waiver excuses a 1.x
// requirement the 2.0 contract replaced, so it names the section that
// supersedes it; a requirement with no successor leaves the registry instead.
func lintWaiver(e Entry, contract contractText) []string {
	var out []string
	bad := func(format string, args ...any) {
		out = append(out, fmt.Sprintf("%s: %s", e.ID, fmt.Sprintf(format, args...)))
	}
	if e.Waiver != nil && !e.Waived() {
		bad("waiver has no reason")
	}
	if !e.Waived() || e.Tier != Tier11 {
		return out
	}
	cited := sectionCitation.FindAllStringSubmatch(e.WaiverReason(), -1)
	if len(cited) == 0 {
		bad("tier %s waiver must cite the superseding CONTRACT §N, or the requirement must leave the registry", Tier11)
	}
	for _, c := range cited {
		if !contract.sections[c[1]] {
			bad("waiver cites CONTRACT §%s, which does not exist", c[1])
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
