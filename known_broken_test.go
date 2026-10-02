package evo_test

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// knownBrokenList is the ZYS-1382 quarantine record.
const knownBrokenList = "docs/zys-1382/KNOWN_BROKEN.md"

// quarantineTag is the first line of every quarantined file.
const quarantineTag = "//go:build evo_pre1382"

// quarantinedSection is the KNOWN_BROKEN.md heading whose bullets name the
// quarantined files.
const quarantinedSection = "## Quarantined files"

var listedFile = regexp.MustCompile("^- `([^`]+\\.go)`")

// TestKnownBroken_ListMatchesQuarantine fails when a file carries the
// evo_pre1382 quarantine tag without a KNOWN_BROKEN.md entry, or the list
// names a file that is no longer quarantined.
func TestKnownBroken_ListMatchesQuarantine(t *testing.T) {
	listed := listedQuarantine(t)
	tagged := taggedQuarantine(t)
	for _, path := range tagged {
		if !slices.Contains(listed, path) {
			t.Errorf("%s is quarantined (%s) but missing from %s", path, quarantineTag, knownBrokenList)
		}
	}
	for _, path := range listed {
		if !slices.Contains(tagged, path) {
			t.Errorf("%s names %s, which does not start with %s", knownBrokenList, path, quarantineTag)
		}
	}
}

// listedQuarantine returns the files the Quarantined section names, sorted.
func listedQuarantine(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile(knownBrokenList)
	if err != nil {
		t.Fatalf("read %s: %v", knownBrokenList, err)
	}
	var listed []string
	inSection := false
	for line := range strings.SplitSeq(string(body), "\n") {
		if strings.HasPrefix(line, "## ") {
			inSection = line == quarantinedSection
			continue
		}
		if m := listedFile.FindStringSubmatch(line); inSection && m != nil {
			listed = append(listed, m[1])
		}
	}
	if len(listed) == 0 {
		t.Fatalf("%s has no %q bullets", knownBrokenList, quarantinedSection)
	}
	slices.Sort(listed)
	return listed
}

// taggedQuarantine returns every module .go file whose first line is the
// quarantine tag, as slash-separated module-relative paths, sorted.
func taggedQuarantine(t *testing.T) []string {
	t.Helper()
	var tagged []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != "." && (strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		first, readErr := firstLine(path)
		if readErr != nil {
			return readErr
		}
		if first == quarantineTag {
			tagged = append(tagged, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk module for %s: %v", quarantineTag, err)
	}
	slices.Sort(tagged)
	return tagged
}

func firstLine(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	sc := bufio.NewScanner(f)
	sc.Scan()
	line, scanErr := sc.Text(), sc.Err()
	if closeErr := f.Close(); scanErr == nil {
		scanErr = closeErr
	}
	return line, scanErr
}
