package main

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeGit struct {
	changes []Change
	diffs   map[string]string
	err     error
}

func (f fakeGit) NameStatus(string) ([]Change, error)  { return f.changes, f.err }
func (f fakeGit) FileDiff(_, p string) (string, error) { return f.diffs[p], nil }

const (
	contractTest = "conformance/contract/x/x_test.go"
	tagRemoval   = "--- a/f\n+++ b/f\n@@ -1,2 +0,0 @@\n-//go:build evopending\n-\n"
)

func run(t *testing.T, g fakeGit, allow string) (int, string) {
	t.Helper()
	var out bytes.Buffer
	code, err := execute("base", allow, g, &out)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	return code, out.String()
}

func TestFrozenPathsAreViolations(t *testing.T) {
	for _, p := range []string{
		"conformance/spec/req/s30.json", "conformance/goldens/a_test.go",
		"testdata/api_golden.txt", "testdata/api_golden_extra.txt",
	} {
		code, out := run(t, fakeGit{changes: []Change{{"M", p}}}, "")
		if code != 1 || !strings.Contains(out, p+": frozen file changed") {
			t.Errorf("%s: code %d out %q", p, code, out)
		}
	}
}

func TestCleanDiffPasses(t *testing.T) {
	code, out := run(t, fakeGit{changes: []Change{{"M", "internal/engine/a.go"}}}, "")
	if code != 0 || out != "" {
		t.Fatalf("code %d out %q", code, out)
	}
}

func TestContractTestMayOnlyLoseBuildTag(t *testing.T) {
	code, _ := run(t, fakeGit{changes: []Change{{"M", contractTest}}, diffs: map[string]string{contractTest: tagRemoval}}, "")
	if code != 0 {
		t.Fatalf("tag removal rejected, code %d", code)
	}
	tagOnly := "-//go:build evopending\n"
	code, _ = run(t, fakeGit{changes: []Change{{"M", contractTest}}, diffs: map[string]string{contractTest: tagOnly}}, "")
	if code != 0 {
		t.Fatalf("tag-only removal rejected, code %d", code)
	}
}

func TestContractTestOtherEditsAreViolations(t *testing.T) {
	cases := map[string]fakeGit{
		"body edit":    {changes: []Change{{"M", contractTest}}, diffs: map[string]string{contractTest: tagRemoval + "-old\n+new\n"}},
		"other delete": {changes: []Change{{"M", contractTest}}, diffs: map[string]string{contractTest: "-foo()\n"}},
		"added line":   {changes: []Change{{"M", contractTest}}, diffs: map[string]string{contractTest: tagRemoval + "+x\n"}},
		"new file":     {changes: []Change{{"A", contractTest}}},
		"deleted file": {changes: []Change{{"D", contractTest}}},
	}
	for name, g := range cases {
		if code, out := run(t, g, ""); code != 1 || !strings.Contains(out, contractTest) {
			t.Errorf("%s: code %d out %q", name, code, out)
		}
	}
}

func ratchetGit(diff string) fakeGit {
	return fakeGit{changes: []Change{{"M", ratchetPath}}, diffs: map[string]string{ratchetPath: diff}}
}

func TestRatchetAddsNeedAllowance(t *testing.T) {
	diff := "+  \"C30-004\",\n+  \"C30-005\",\n"
	code, out := run(t, ratchetGit(diff), "C30-004")
	if code != 1 || !strings.Contains(out, "C30-005: ratchet ID added but not in --allow-ids") || strings.Contains(out, "C30-004") {
		t.Fatalf("code %d out %q", code, out)
	}
	if code, _ := run(t, ratchetGit(diff), "C30-004, C30-005"); code != 0 {
		t.Fatalf("allowed adds rejected, code %d", code)
	}
}

func TestRatchetRemovalIsViolation(t *testing.T) {
	code, out := run(t, ratchetGit("-  \"C30-001\",\n"), "C30-001")
	if code != 1 || !strings.Contains(out, "C30-001: ratchet ID removed") {
		t.Fatalf("code %d out %q", code, out)
	}
}

func TestRatchetReformatOfSameIDIsNeutral(t *testing.T) {
	diff := "-  \"C30-001\"\n+  \"C30-001\",\n"
	if code, out := run(t, ratchetGit(diff), ""); code != 0 {
		t.Fatalf("code %d out %q", code, out)
	}
}

func TestBaseIsRequired(t *testing.T) {
	if _, err := execute("", "", fakeGit{}, &bytes.Buffer{}); err == nil {
		t.Fatal("want error for missing --base")
	}
}

func TestGitFailurePropagates(t *testing.T) {
	if _, err := execute("b", "", fakeGit{err: errors.New("boom")}, &bytes.Buffer{}); err == nil {
		t.Fatal("want error")
	}
}

func TestParseNameStatusExpandsRenames(t *testing.T) {
	got := parseNameStatus("M\ta.go\nR087\told.json\tnew.json\n")
	want := []Change{{"M", "a.go"}, {"R", "old.json"}, {"R", "new.json"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
