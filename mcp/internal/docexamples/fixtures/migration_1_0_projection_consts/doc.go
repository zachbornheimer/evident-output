// Package main is the buildable half of this fixture. main.go carries a
// `//go:build ignore` tag (it pins elided, non-compiling doc text — see
// its own header comment), so this file exists purely so the directory
// still has a package for `go build ./...`/`go vet ./...` to see and a
// func main for that package to link. It has no marked snippet region and
// contributes nothing to TestDocFencesMatchFixtures.
package main

func main() {}
