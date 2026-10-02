package main

import (
	"maps"
	"strings"
	"testing"
)

const testContract = "# Contract\n\nThe   quick brown\nfox jumps.\n"

func lintWith(t *testing.T, registry string, extra map[string]string) []string {
	t.Helper()
	files := map[string]string{
		"r/" + contractPath:               testContract,
		"r/conformance/spec/req/s01.json": registry,
	}
	maps.Copy(files, extra)
	out, err := lintRegistry(newFakeFS(files), "r")
	if err != nil {
		t.Fatalf("lintRegistry: %v", err)
	}
	return out
}

func wantViolation(t *testing.T, got []string, substr string) {
	t.Helper()
	for _, line := range got {
		if strings.Contains(line, substr) {
			return
		}
	}
	t.Fatalf("want a violation containing %q, got %v", substr, got)
}

const validEntry = `{"id":"C01-001","section":"1","quote":"quick brown fox","tier":"1.1","tests":[{"pkg":"./x","run":"TestX"}]}`

func TestLintCleanRegistry(t *testing.T) {
	if got := lintWith(t, "["+validEntry+"]", nil); len(got) != 0 {
		t.Fatalf("want clean, got %v", got)
	}
}

func TestLintQuoteCollapsesWhitespace(t *testing.T) {
	entry := `[{"id":"C01-001","section":"1","quote":"The quick  brown fox\njumps.","tier":"1.1","tests":[{"pkg":"./x","run":"TestX"}]}]`
	if got := lintWith(t, entry, nil); len(got) != 0 {
		t.Fatalf("want clean, got %v", got)
	}
}

func TestLintViolations(t *testing.T) {
	cases := map[string]struct{ entry, want string }{
		"quote absent": {`[{"id":"C01-001","section":"1","quote":"not there","tier":"1.1","tests":[{"pkg":"./x","run":"T"}]}]`, "quote does not occur"},
		"bad id":       {`[{"id":"X1","section":"1","quote":"quick","tier":"1.1","tests":[{"pkg":"./x","run":"T"}]}]`, "id does not match"},
		"missing tier": {`[{"id":"C01-001","section":"1","quote":"quick","tests":[{"pkg":"./x","run":"T"}]}]`, "tier"},
		"no tests":     {`[{"id":"C01-001","section":"1","quote":"quick","tier":"1.2"}]`, "at least one test or a waiver"},
		"empty run":    {`[{"id":"C01-001","section":"1","quote":"quick","tier":"1.2","tests":[{"pkg":"./x","run":""}]}]`, "both pkg and run"},
		"empty pkg":    {`[{"id":"C01-001","section":"1","quote":"quick","tier":"1.2","tests":[{"pkg":"","run":"T"}]}]`, "both pkg and run"},
		"duplicate":    {"[" + validEntry + "," + validEntry + "]", "duplicate id"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) { wantViolation(t, lintWith(t, tc.entry, nil), tc.want) })
	}
}

func TestLintWaiverSatisfiesBinding(t *testing.T) {
	entry := `[{"id":"C01-001","section":"1","quote":"quick","tier":"1.2","waiver":"manual only"}]`
	if got := lintWith(t, entry, nil); len(got) != 0 {
		t.Fatalf("want clean, got %v", got)
	}
}

func TestLintDuplicateAcrossFiles(t *testing.T) {
	got := lintWith(t, "["+validEntry+"]", map[string]string{"r/conformance/spec/req/s02.json": "[" + validEntry + "]"})
	wantViolation(t, got, "duplicate id")
}

func TestLintBansSkipInContractTests(t *testing.T) {
	for _, call := range []string{"t.Skip(", "t.Skipf(", "t.SkipNow("} {
		extra := map[string]string{"r/" + contractTests + "/a/a_test.go": "func TestA(t *testing.T) { " + call + ") }"}
		wantViolation(t, lintWith(t, "["+validEntry+"]", extra), "t.Skip is banned")
	}
}
