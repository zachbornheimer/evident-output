package main

import (
	"slices"
	"testing"
)

func TestPatchPathsListsEveryWrittenFile(t *testing.T) {
	patch := []byte("diff --git a/b.go b/b.go\n--- a/b.go\t2026-10-14\n+++ b/b.go\t2026-10-14\n@@ -1 +1 @@\n-x\n+y\n" +
		"--- a/dir/a.go\n+++ b/dir/a.go\n@@ -1 +1 @@\n-x\n+y\n" +
		"--- a/gone.go\n+++ /dev/null\n@@ -1 +0,0 @@\n-x\n" +
		"--- a/b.go\n+++ b/b.go\n@@ -3 +3 @@\n-x\n+y\n")
	want := []string{"b.go", "dir/a.go", "gone.go"}
	if got := patchPaths(patch); !slices.Equal(got, want) {
		t.Fatalf("patchPaths = %v, want %v", got, want)
	}
}

func TestOutsideFixNamesFilesTheFixDidNotChange(t *testing.T) {
	fix := []string{"a.go", "a_test.go", "b.go"}
	cases := map[string]struct{ patched, want []string }{
		"subset":          {[]string{"a.go", "b.go"}, nil},
		"unrelated file":  {[]string{"a.go", "c.go"}, []string{"c.go"}},
		"fix's test file": {[]string{"a_test.go"}, []string{"a_test.go"}},
		"empty patch":     {nil, nil},
	}
	for label, c := range cases {
		if got := outsideFix(c.patched, fix); !slices.Equal(got, c.want) {
			t.Errorf("%s: outsideFix = %v, want %v", label, got, c.want)
		}
	}
}
