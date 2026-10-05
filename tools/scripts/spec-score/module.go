package main

import "path/filepath"

const goModFile = "go.mod"

// testTarget is where one package's tests must run: the module directory to
// use as the working directory, and the package path relative to it.
type testTarget struct {
	Dir string
	Pkg string
}

// resolveTestTarget finds the nearest go.mod at or above the package
// directory, stopping below root. A package in a nested module only
// builds with that module as the working directory; every other package
// runs from root unchanged.
func resolveTestTarget(fsys FileSystem, root, pkg string) testTarget {
	root = filepath.Clean(root)
	pkgDir := filepath.Join(root, filepath.FromSlash(pkg))
	for dir := pkgDir; dir != root && dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		matches, err := fsys.Glob(filepath.Join(dir, goModFile))
		if err != nil || len(matches) == 0 {
			continue
		}
		rel, err := filepath.Rel(dir, pkgDir)
		if err != nil {
			break
		}
		if rel == "." {
			return testTarget{Dir: dir, Pkg: "."}
		}
		return testTarget{Dir: dir, Pkg: "./" + filepath.ToSlash(rel)}
	}
	return testTarget{Dir: root, Pkg: pkg}
}
