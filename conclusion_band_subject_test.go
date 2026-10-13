package evo

import (
	"bytes"
	"strings"
	"testing"
)

// TestConclusionBand_NoTitleNeverStutters is the release-gate cosmetic
// finding (round 10): when no Config.Title is configured, writeConclusion
// fell back to the bare state word as the printed Subject — so the band's
// tag and its subject line said the same word twice ("[changed]  changed",
// "[failed]  failed"). The subject line must be suppressed instead of
// stuttering; only the bracketed tag carries the state word.
func TestConclusionBand_NoTitleNeverStutters(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &buf, color: colorNever, plain: true})

	branches := out.Task("branches")
	branches.Define(effectOf(Delete, "stale local branch", 3))
	if err := out.finish(); err != nil {
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
