package main

import (
	"strings"
	"testing"
)

func events(lines ...string) []byte { return []byte(strings.Join(lines, "\n") + "\n") }

func TestVerdictsClassifyEachOutcome(t *testing.T) {
	out := events(
		`{"Action":"run","Test":"TestRed"}`, `{"Action":"fail","Test":"TestRed"}`,
		`{"Action":"run","Test":"TestGreen"}`, `{"Action":"pass","Test":"TestGreen"}`,
		`{"Action":"run","Test":"TestSkip"}`, `{"Action":"skip","Test":"TestSkip"}`,
		`{"Action":"run","Test":"TestKilled"}`,
		`not json at all`,
	)
	got := Verdicts(out, []string{"TestRed", "TestGreen", "TestSkip", "TestKilled", "TestMissing"})
	want := map[string]Verdict{"TestRed": Red, "TestGreen": Green, "TestSkip": Skipped, "TestKilled": Red, "TestMissing": NotRun}
	for name, v := range want {
		if got[name] != v {
			t.Errorf("%s = %s, want %s", name, got[name], v)
		}
	}
}

func TestVerdictsIgnoreSubtestsOfOtherNames(t *testing.T) {
	out := events(`{"Action":"run","Test":"TestA/sub"}`, `{"Action":"fail","Test":"TestA/sub"}`)
	if got := Verdicts(out, []string{"TestA"})["TestA"]; got != NotRun {
		t.Errorf("TestA = %s, want not-run (only a subtest reported)", got)
	}
}

func TestPackageOutputKeepsBuildFailureText(t *testing.T) {
	out := events(`{"Action":"build-output","Output":"undefined: Plan\n"}`, `{"Action":"output","Test":"TestX","Output":"noise\n"}`)
	if got := PackageOutput(out); got != "undefined: Plan" {
		t.Errorf("PackageOutput = %q", got)
	}
}
