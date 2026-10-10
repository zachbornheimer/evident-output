package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/process"
)

const (
	// wholeModule runs every package: a root file can change any of them.
	wholeModule = "./..."
	// rootPackage is the module's root package, which owns the tests that
	// read repository files (layout, docs) no Go package owns.
	rootPackage = "."
	// testdataDir holds fixtures, never a package.
	testdataDir = "testdata"
)

// commitTestPackages returns the packages whose tests a commit can turn red:
// the owners of the files it changed against its first parent. The worktree
// must have the commit checked out.
func commitTestPackages(worktree, sha string) ([]string, error) {
	out, err := runGit(worktree, "show", "--first-parent", "-m", "--name-only", "-z", "--format=", sha)
	if err != nil {
		return nil, fmt.Errorf("list files changed by %s: %w", sha, err)
	}
	changed := slices.DeleteFunc(strings.Split(out, "\x00"), func(name string) bool {
		return strings.TrimSpace(name) == ""
	})
	return testPackagesFor(changed, directoryHasGoFiles(worktree)), nil
}

// testPackagesFor maps changed files (slash-separated, repo-relative) to the
// sorted, de-duplicated go test package arguments that cover them. A file in
// the repository root maps to wholeModule alone.
func testPackagesFor(changed []string, hasGoFiles func(dir string) bool) []string {
	owners := map[string]bool{}
	for _, file := range changed {
		dir := path.Dir(file)
		if dir == "." {
			return []string{wholeModule}
		}
		owners[owningPackage(beforeTestdata(dir), hasGoFiles)] = true
	}
	packages := make([]string, 0, len(owners))
	for pkg := range owners {
		packages = append(packages, pkg)
	}
	slices.Sort(packages)
	return packages
}

// owningPackage is the nearest directory at or above dir that holds Go files,
// as a go test argument. A directory deleted by the commit, or one that only
// holds documents, falls up to rootPackage.
func owningPackage(dir string, hasGoFiles func(dir string) bool) string {
	for dir != "." && !hasGoFiles(dir) {
		dir = path.Dir(dir)
	}
	if dir == "." {
		return rootPackage
	}
	return "./" + dir
}

// beforeTestdata trims dir at its first testdata segment, so a fixture maps
// to the package that owns the fixtures.
func beforeTestdata(dir string) string {
	segments := strings.Split(dir, "/")
	cut := slices.Index(segments, testdataDir)
	if cut < 0 {
		return dir
	}
	if cut == 0 {
		return "."
	}
	return path.Join(segments[:cut]...)
}

func directoryHasGoFiles(worktree string) func(dir string) bool {
	return func(dir string) bool {
		entries, err := os.ReadDir(filepath.Join(worktree, filepath.FromSlash(dir)))
		if err != nil {
			return false
		}
		return slices.ContainsFunc(entries, func(e os.DirEntry) bool {
			return !e.IsDir() && strings.HasSuffix(e.Name(), ".go")
		})
	}
}

func testPackages(worktree string, packages []string) (string, error) {
	cmd := process.NewCmd("go", append([]string{"test"}, packages...)...)
	cmd.SetDir(worktree)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
