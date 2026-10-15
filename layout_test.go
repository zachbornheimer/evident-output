package evo

import (
	"fmt"
	"go/scanner"
	"go/token"
	"os"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
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

// statedExamples matches the program count LAYOUT.md's examples/ row states.
var statedExamples = regexp.MustCompile("`examples/`\\s*\\|\\s*(\\d+) runnable demo programs")

// TestLayoutExamplesCountMatches fails when the number of demo programs
// LAYOUT.md states differs from the directories under examples/ that hold a
// main.go, so the prose cannot drift from the tree.
func TestLayoutExamplesCountMatches(t *testing.T) {
	body, err := os.ReadFile("LAYOUT.md")
	if err != nil {
		t.Fatal(err)
	}
	m := statedExamples.FindStringSubmatch(string(body))
	if m == nil {
		t.Fatal("LAYOUT.md: no `examples/` row stating \"N runnable demo programs\"")
	}
	stated, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir("examples")
	if err != nil {
		t.Fatal(err)
	}
	programs := 0
	for _, entry := range entries {
		if _, err := os.Stat(path.Join("examples", entry.Name(), "main.go")); err == nil {
			programs++
		}
	}
	if stated != programs {
		t.Errorf("LAYOUT.md states %d runnable demo programs; examples/ holds %d", stated, programs)
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
// that belongs to the internal package that owns it. It budgets code lines
// only, so documentation never competes with structure.
const rootLineBudget = 4000

// foldedSeam matches the marker of code folded into the root from a package
// it absorbed.
var foldedSeam = regexp.MustCompile(`// --- folded\sfrom `)

// codeLines counts the lines of a Go source that carry at least one token
// other than a comment: blank lines and comment-only lines are not code.
func codeLines(t *testing.T, name string, body []byte) int {
	t.Helper()
	fset := token.NewFileSet()
	file := fset.AddFile(name, fset.Base(), len(body))
	var s scanner.Scanner
	s.Init(file, body, func(pos token.Position, msg string) {
		t.Fatalf("scan %s at %s: %s", name, pos, msg)
	}, 0)
	lines := map[int]bool{}
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.SEMICOLON && lit == "\n" {
			continue
		}
		first := file.Line(pos)
		last := first + strings.Count(lit, "\n")
		for line := first; line <= last; line++ {
			lines[line] = true
		}
	}
	return len(lines)
}

// TestRootIsThin fails when the root package's non-test Go files reach
// rootLineBudget code lines together (blank and comment lines are not
// counted), or any of them carries a folded-from seam.
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
		lines := codeLines(t, name, body)
		total += lines
		sizes = append(sizes, fmt.Sprintf("%s %d", name, lines))
		if foldedSeam.Match(body) {
			t.Errorf("%s carries a folded-from seam; merge it into the file that owns it", name)
		}
	}
	if total >= rootLineBudget {
		t.Errorf("the root package holds %d non-test code lines (non-blank, non-comment), want under %d:\n  %s",
			total, rootLineBudget, strings.Join(sizes, "\n  "))
	}
	t.Logf("root package code lines: %d (budget %d)", total, rootLineBudget)
}
