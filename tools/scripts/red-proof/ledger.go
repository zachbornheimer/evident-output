package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// TestRef names one guarding test: the package directory (as `go test` takes
// it, relative to the module root) and the top-level test function.
type TestRef struct {
	Pkg  string
	Name string
}

// Entry records one fix commit and the tests that must fail without it.
type Entry struct {
	Commit string
	Note   string
	// Patch names the hand-written patch, relative to the ledger, that
	// re-introduces the bug in HEAD. Reversing the commit's own hunks is not
	// an option: later edits keep them from applying. It may patch only
	// non-test files the fix commit changed (TestLedgerPatchTouchesOnlyTheFixCommitsFiles).
	Patch string
	Tests []TestRef
}

// Ledger is every fix commit whose guarding tests the tool can replay.
type Ledger struct{ Entries []Entry }

var (
	commitPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	pkgPattern    = regexp.MustCompile(`^\.(/[A-Za-z0-9_-]+)*$`)
	testPattern   = regexp.MustCompile(`^Test[A-Za-z0-9_]*$`)
	patchPattern  = regexp.MustCompile(`^patches/[A-Za-z0-9_-]+\.patch$`)
)

// ParseLedger reads the ledger's fixed YAML subset: an `entries:` list whose
// items carry `commit`, `note`, a `patch` path, and a
// `tests:` list of `pkg`/`name` pairs.
// Anything else is an error, so a typo cannot silently drop a guard.
func ParseLedger(text string) (Ledger, error) {
	var ledger Ledger
	for n, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		item := strings.HasPrefix(line, "- ")
		key, value, found := strings.Cut(strings.TrimPrefix(line, "- "), ":")
		if !found {
			return Ledger{}, fmt.Errorf("ledger line %d: want `key: value`, got %q", n+1, line)
		}
		if err := ledger.apply(item, strings.TrimSpace(key), unquote(strings.TrimSpace(value))); err != nil {
			return Ledger{}, fmt.Errorf("ledger line %d: %w", n+1, err)
		}
	}
	return ledger, ledger.Validate()
}

func (l *Ledger) apply(item bool, key, value string) error {
	last := len(l.Entries) - 1
	switch {
	case key == "entries" && !item && value == "":
		return nil
	case key == "commit" && item:
		l.Entries = append(l.Entries, Entry{Commit: value})
	case key == "note" && !item && last >= 0:
		l.Entries[last].Note = value
	case key == "patch" && !item && last >= 0:
		l.Entries[last].Patch = value
	case key == "tests" && !item && last >= 0 && value == "":
		return nil
	case key == "pkg" && item && last >= 0:
		l.Entries[last].Tests = append(l.Entries[last].Tests, TestRef{Pkg: value})
	case key == "name" && !item && last >= 0 && len(l.Entries[last].Tests) > 0:
		tests := l.Entries[last].Tests
		tests[len(tests)-1].Name = value
	default:
		return fmt.Errorf("unexpected %q (item=%t)", key, item)
	}
	return nil
}

func unquote(value string) string {
	if s, err := strconv.Unquote(value); err == nil {
		return s
	}
	return value
}

// Validate rejects entries the runner could not replay safely: malformed
// commits, patches and test references, entries without tests, and duplicates.
func (l Ledger) Validate() error {
	if len(l.Entries) == 0 {
		return fmt.Errorf("ledger has no entries")
	}
	seen := map[string]bool{}
	for _, e := range l.Entries {
		if !commitPattern.MatchString(e.Commit) {
			return fmt.Errorf("entry %q: commit must be 7-40 lowercase hex digits", e.Commit)
		}
		if seen[e.Commit] {
			return fmt.Errorf("entry %s: listed twice", e.Commit)
		}
		seen[e.Commit] = true
		if !patchPattern.MatchString(e.Patch) {
			return fmt.Errorf("entry %s: patch %q must be patches/<name>.patch", e.Commit, e.Patch)
		}
		if len(e.Tests) == 0 {
			return fmt.Errorf("entry %s: names no guarding test", e.Commit)
		}
		for _, t := range e.Tests {
			if !pkgPattern.MatchString(t.Pkg) || !testPattern.MatchString(t.Name) {
				return fmt.Errorf("entry %s: bad test reference pkg=%q name=%q", e.Commit, t.Pkg, t.Name)
			}
		}
	}
	return nil
}

// PackageTests groups an entry's tests by package, in first-seen order.
func (e Entry) PackageTests() (pkgs []string, names map[string][]string) {
	names = map[string][]string{}
	for _, t := range e.Tests {
		if _, ok := names[t.Pkg]; !ok {
			pkgs = append(pkgs, t.Pkg)
		}
		names[t.Pkg] = append(names[t.Pkg], t.Name)
	}
	return pkgs, names
}
