package evo

import (
	"testing"

	txt "github.com/zachbornheimer/evident-output/v2/internal/project"
)

// The words a conclusion band names a run by (see txt.ConclusionHeadline).
const (
	headlineSucceeded = string(Succeeded)
	headlineSatisfied = string(Satisfied)
	headlineExcluded  = string(Excluded)
	headlineRefused   = string(Refused)
	headlineFailed    = string(Failed)
	headlineCancelled = string(Cancelled)
	headlineChanged   = txt.HeadlineChanged
	headlinePlanned   = txt.HeadlinePlanned
)

// headline is the word c's conclusion band names the run by.
func (c Conclusion) headline() string {
	return txt.ConclusionHeadline(asRecordConclusion(c))
}

func requireConclusion(t *testing.T, out *Run, want string) {
	t.Helper()
	got := out.conclusion()
	if got.headline() != want {
		t.Fatalf("conclusion headline = %q, want %q", got.headline(), want)
	}
}

func requireClean(t *testing.T, out *Run) {
	t.Helper()
	if err := out.err(); err != nil {
		t.Fatalf("output misuse error: %v", err)
	}
}
