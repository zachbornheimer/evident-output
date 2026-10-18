package main

import (
	"slices"
	"strings"
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

func TestPatchScopeFollowsCodeMovedOutOfAFixFile(t *testing.T) {
	patch := []byte("--- a/result.go\n+++ b/result.go\n@@ -2,3 +2,3 @@\n ctx\n-old\n+new\n ctx\n")
	addOnly := []byte("--- a/result.go\n+++ b/result.go\n@@ -2,1 +2,2 @@\n ctx\n+new\n")
	testPatch := []byte("--- a/result_test.go\n+++ b/result_test.go\n@@ -1,1 +1,1 @@\n-old\n+new\n")
	fix := []string{"report.go"}
	cases := map[string]struct {
		patch  []byte
		origin string
		want   []string
	}{
		"moved lines":   {patch, "report.go", nil},
		"foreign lines": {patch, "other.go", []string{"result.go"}},
		"added only":    {addOnly, "report.go", []string{"result.go"}},
		"test file":     {testPatch, "report.go", []string{"result_test.go"}},
	}
	for label, c := range cases {
		git := fakeGit{head: "a\nb\nctx\nold\nctx\n", origins: map[string]string{"result.go": c.origin, "result_test.go": c.origin}}
		got, err := outsideFixFollowingMoves(git, c.patch, fix)
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%s: outside = %v (err %v), want %v", label, got, err, c.want)
		}
	}
}

func TestPatchRemovedLinesNumbersLinesInThePreImage(t *testing.T) {
	patch := []byte("--- a/a.go\n+++ b/a.go\n@@ -5,4 +5,3 @@\n ctx\n-x\n-y\n+z\n ctx\n@@ -20 +19 @@\n-q\n+r\n")
	// The file lags the stated start by two lines, as a stale patch does.
	head := strings.Repeat("pad\n", 6) + "ctx\nx\ny\nctx\n" + strings.Repeat("pad\n", 10) + "q\n"
	got, err := patchRemovedLines(patch, func(string) ([]byte, error) { return []byte(head), nil })
	if err != nil || !slices.Equal(got["a.go"], []int{8, 9, 21}) {
		t.Fatalf("removed = %v (err %v), want [8 9 21]", got, err)
	}
}
