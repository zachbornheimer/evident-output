package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestConclusionBand_NoTitleNeverStutters is the release-gate cosmetic
// finding (round 10): when no Config.Title is configured, writeConclusion
// fell back to the bare state word as the printed Subject — so the band's
// tag and its subject line said the same word twice ("[changed]  changed",
// "[failed]  failed"). The subject line must be suppressed instead of
// stuttering; only the bracketed tag carries the state word.
func TestConclusionBand_NoTitleNeverStutters(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})

	branches := out.Task("branches")
	branches.Delete(3, "stale local branch")
	branches.Done()
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil", err)
	}

	got := buf.String()
	if !strings.Contains(got, "[changed]") {
		t.Fatalf("want [changed] band, got:\n%s", got)
	}
	if strings.Contains(got, "[changed]  changed") {
		t.Fatalf("band subject stutters the bare state word, got:\n%s", got)
	}
}
