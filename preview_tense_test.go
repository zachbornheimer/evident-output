package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestPreview_PlannedTenseWithoutTheDryRunTag is the red-first proof for the
// canary's first finding: `zq clean-repo` without `--dry-run` opened with
// `[dry-run] zq clean-repo <path>` and then asked permission to apply. That
// is a preview before a confirm gate, not a dry run. The dialect's
// screenshot-regression item 1 says so outright: "Do not label it dry-run."
//
// Config.Preview is the same planned tense as Config.DryRun — mutation
// callbacks never run, effects land in the `[planned]` ledger — with the
// dialect's plain `repo <path>` header instead of the tagged one, and the
// same redundant-band suppression on a pure planned verdict.
func TestPreview_PlannedTenseWithoutTheDryRunTag(t *testing.T) {
	t.Parallel()
	called := false
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Color: evo.ColorNever,
		Title: "zq", Subject: "repo  /Users/zach/dev/flight", Preview: true,
		Stdout: &buf,
	})
	branches := out.Task("branches")
	branches.Delete("local tip", func() error {
		called = true
		return nil
	}, evo.Affected(8))
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	got := buf.String()
	if called {
		t.Fatalf("a preview must never run the mutation callback:\n%s", got)
	}
	if strings.Contains(got, "[dry-run]") {
		t.Fatalf("a preview before a confirm gate is not a dry run:\n%s", got)
	}
	if !strings.HasPrefix(got, "repo  /Users/zach/dev/flight\n\n") {
		t.Fatalf("want the plain subject header first:\n%q", got)
	}
	if !strings.Contains(got, "[planned] branches") {
		t.Fatalf("want the planned ledger:\n%s", got)
	}
	if strings.Contains(got, "[changed]") {
		t.Fatalf("a preview never uses committed tense:\n%s", got)
	}
	if strings.Contains(got, "[planned]  zq") {
		t.Fatalf("a pure planned verdict under a rendered header needs no band:\n%s", got)
	}
}

// TestDryRun_KeepsItsTag guards the blast radius: Config.DryRun is unchanged.
func TestDryRun_KeepsItsTag(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Color: evo.ColorNever,
		Title: "zq", Subject: "repo  /Users/zach/dev/flight", DryRun: true,
		Stdout: &buf,
	})
	branches := out.Task("branches")
	branches.Delete("local tip", func() error { return nil }, evo.Affected(8))
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	if got := buf.String(); !strings.HasPrefix(got, "[dry-run] repo  /Users/zach/dev/flight\n\n") {
		t.Fatalf("DryRun keeps its tag:\n%q", got)
	}
}
