// Package main is the buildable half of this fixture. main.go carries a
// `//go:build ignore` tag so the fence can pin 1.x doc text, so this file
// exists purely so the directory still has a package for go vet to see.
package main

func main() {}
