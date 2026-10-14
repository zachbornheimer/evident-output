package main

import (
	"maps"
	"strings"
	"testing"
)

const testContract = "# Contract\n\n## 1. Run\n\nThe   quick brown\nfox jumps.\n"

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

func TestLintWaiverAllowsHistoricalQuote(t *testing.T) {
	entry := `[{"id":"C01-001","section":"1","quote":"not in the 2.0 contract","tier":"1.1","waiver":"CONTRACT §1 replaced this 1.x sentence"}]`
	if got := lintWith(t, entry, nil); len(got) != 0 {
		t.Fatalf("want clean, got %v", got)
	}
}

func TestLintWaiverNeedsAReason(t *testing.T) {
	for name, waiver := range map[string]string{"empty": `""`, "blank": `"  \n"`} {
		t.Run(name, func(t *testing.T) {
			entry := `[{"id":"C01-001","section":"1","quote":"quick","tier":"1.2","tests":[{"pkg":"./x","run":"T"}],"waiver":` + waiver + `}]`
			wantViolation(t, lintWith(t, entry, nil), "waiver has no reason")
		})
	}
}

func TestLintTier11WaiverCitesASection(t *testing.T) {
	cases := map[string]struct{ waiver, want string }{
		"no citation":       {"superseded: the contract does not carry this", "must cite the superseding CONTRACT §N"},
		"missing section":   {"CONTRACT §9 replaced this", "CONTRACT §9, which does not exist"},
		"bare section sign": {"see §1", "must cite the superseding CONTRACT §N"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			entry := `[{"id":"C01-001","section":"1","quote":"gone","tier":"1.1","waiver":"` + tc.waiver + `"}]`
			wantViolation(t, lintWith(t, entry, nil), tc.want)
		})
	}
}

func TestLintTier11WaiverCitingASectionIsClean(t *testing.T) {
	for _, waiver := range []string{"CONTRACT §1 replaced this 1.x sentence", "removed in 2.0: Skipped (now Exclude, CONTRACT.md §1)"} {
		entry := `[{"id":"C01-001","section":"1","quote":"gone","tier":"1.1","waiver":"` + waiver + `"}]`
		if got := lintWith(t, entry, nil); len(got) != 0 {
			t.Fatalf("waiver %q: want clean, got %v", waiver, got)
		}
	}
}

func TestLintTier12WaiverNeedsNoCitation(t *testing.T) {
	entry := `[{"id":"C01-001","section":"1","quote":"gone","tier":"1.2","waiver":"manual review only"}]`
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
