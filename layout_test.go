package evo_test

import (
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
