package evo

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"slices"
	"sort"
	"strings"
	"testing"
)

// TestLayoutIsExact fails when a tracked file lives outside the layout block
// in LAYOUT.md, or when a listed path is missing. A bare entry is one file; an
// entry ending in "/" is a directory that must hold at least one tracked file
// and may hold anything beneath it. Root *_test.go files and a root testdata/
// directory are the only unlisted paths allowed.
func TestLayoutIsExact(t *testing.T) {
	files, dirs := layoutEntries(t, "LAYOUT.md")
	tracked := trackedFiles(t)

	var stray, missing []string
	for _, f := range tracked {
		if !allowed(f, files, dirs) {
			stray = append(stray, f)
		}
	}
	for f := range files {
		if !containsString(tracked, f) {
			missing = append(missing, f)
		}
	}
	for d := range dirs {
		if !anyUnder(tracked, d) {
			missing = append(missing, d)
		}
	}
	sort.Strings(stray)
	sort.Strings(missing)

	if len(missing) > 0 {
		t.Errorf("LAYOUT.md lists %d paths the repository lacks:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
	if len(stray) > 0 {
		t.Errorf("%d tracked files live outside LAYOUT.md:\n  %s",
			len(stray), strings.Join(stray, "\n  "))
	}
}

func layoutEntries(t *testing.T, file string) (files, dirs map[string]bool) {
	t.Helper()
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	files, dirs = map[string]bool{}, map[string]bool{}
	inBlock := false
	for line := range strings.SplitSeq(string(body), "\n") {
		switch {
		case strings.HasPrefix(line, "```layout"):
			inBlock = true
		case inBlock && strings.HasPrefix(line, "```"):
			inBlock = false
		case inBlock && strings.TrimSpace(line) != "":
			entry := strings.TrimSpace(line)
			if before, ok := strings.CutSuffix(entry, "/"); ok {
				dirs[before] = true
			} else {
				files[entry] = true
			}
		}
	}
	if len(files)+len(dirs) == 0 {
		t.Fatalf("%s: no ```layout block found", file)
	}
	return files, dirs
}

func trackedFiles(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("git", "ls-files", "-z").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	var files []string
	for f := range strings.SplitSeq(string(out), "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files
}

func allowed(file string, files, dirs map[string]bool) bool {
	if files[file] {
		return true
	}
	if path.Dir(file) == "." && strings.HasSuffix(file, "_test.go") {
		return true
	}
	if strings.HasPrefix(file, "testdata/") {
		return true
	}
	for dir := range dirs {
		if strings.HasPrefix(file, dir+"/") {
			return true
		}
	}
	return false
}

func anyUnder(files []string, dir string) bool {
	for _, f := range files {
		if strings.HasPrefix(f, dir+"/") {
			return true
		}
	}
	return false
}

func containsString(list []string, want string) bool {
	return slices.Contains(list, want)
}

// rootLineBudget is where the root package stops being thin: it holds the
// public surface and the composition root (LAYOUT.md), and every line past
// that belongs to the internal package that owns it.
const rootLineBudget = 4000

// foldedSeam marks code folded into the root from a package it absorbed.
const foldedSeam = "// --- folded from "

// TestRootIsThin fails when the root package's non-test Go files reach
// rootLineBudget lines together, or any of them carries a folded-from seam.
func TestRootIsThin(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the root package: %v", err)
	}
	total := 0
	var sizes []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		lines := strings.Count(string(body), "\n")
		total += lines
		sizes = append(sizes, fmt.Sprintf("%s %d", name, lines))
		if strings.Contains(string(body), foldedSeam) {
			t.Errorf("%s carries a folded-from seam; merge it into the file that owns it", name)
		}
	}
	if total >= rootLineBudget {
		t.Errorf("the root package holds %d non-test lines, want under %d:\n  %s",
			total, rootLineBudget, strings.Join(sizes, "\n  "))
	}
}
