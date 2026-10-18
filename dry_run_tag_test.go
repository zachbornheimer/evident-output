package evo

import (
	"bytes"
	"strings"
	"testing"
)

// TestDryRun_KeepsItsTag guards the blast radius: Config.DryRun is unchanged.
func TestDryRun_KeepsItsTag(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := initRun(Config{Name: "zq", DryRun: true, Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	branches := out.Task("branches")
	branches.Define(effectOf(Delete, "local tip", 8))
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	if got := buf.String(); !strings.HasPrefix(got, "[dry-run] no changes will be made\n\n") {
		t.Fatalf("DryRun keeps its tag:\n%q", got)
	}
}
