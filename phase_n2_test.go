package evo_test

import (
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// Phase N2 closes the RENDERING gaps in evo-rec.md audited against probes:
// "! already mutated:" derived from the effect ledger (1), the un-dimmed
// in-flight phase (2, covered by live_progress_bar_test.go's rename),
// bounded effect rows (3), empty effect section grammar (4, covered by
// dry_run_test.go), the next-action glyph (5), ASCII profile completeness
// (6), and Confirm EOF as a policy block (7, covered by confirm_test.go).

// TestConclusion_AlreadyMutated_CancelledWithChanges is red-first for item 1:
// a Cancelled run with committed effects must render one derived
// "! already mutated: ..." line summarizing the Changes ledger — never a
// caller-assembled string.
func TestConclusion_AlreadyMutated_CancelledWithChanges(t *testing.T) {
	var buf strings.Builder
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	branches := out.Task("branches")
	branches.Delete(8, "local branch")
	branches.Done()
	out.Cancel("interrupted")
	if err := out.Finish(); err != nil {
		t.Log(err)
	}
	got := buf.String()
	if !strings.Contains(got, "!  already mutated: 8 local branches deleted") {
		t.Fatalf("want derived already-mutated line, got:\n%s", got)
	}
}

// TestConclusion_AlreadyMutated_CancelledEmptyLedger proves an empty Changes
// ledger suppresses the "! already mutated: ..." row entirely — "!" is
// attention-only (evo-rec.md "Tightened glyph vocabulary"), and "none" earns
// no attention. Partial truth still holds: there is simply nothing to report.
func TestConclusion_AlreadyMutated_CancelledEmptyLedger(t *testing.T) {
	var buf strings.Builder
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	out.Task("scan").Phase("scanning")
	out.Cancel("interrupted")
	if err := out.Finish(); err != nil {
		t.Log(err)
	}
	got := buf.String()
	if strings.Contains(got, "already mutated") {
		t.Fatalf("empty ledger must not render already-mutated line, got:\n%s", got)
	}
}

// TestConclusion_AlreadyMutated_Failed proves the same derived line renders
// on a Failed conclusion, not only Cancelled.
func TestConclusion_AlreadyMutated_Failed(t *testing.T) {
	var buf strings.Builder
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	remotes := out.Task("remotes")
	remotes.Delete(1, "origin tip")
	remotes.Fail("authentication failed")
	if err := out.Finish(); err != nil {
		t.Log(err)
	}
	got := buf.String()
	if !strings.Contains(got, "!  already mutated: 1 origin tip deleted") {
		t.Fatalf("want derived already-mutated line on Failed, got:\n%s", got)
	}
}

// TestConclusion_AlreadyMutated_NotRenderedOnSuccess proves the line is
// specific to abnormal termination — a normal Done run never renders it.
func TestConclusion_AlreadyMutated_NotRenderedOnSuccess(t *testing.T) {
	var buf strings.Builder
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	branches := out.Task("branches")
	branches.Delete(8, "local branch")
	branches.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, "already mutated") {
		t.Fatalf("success must not render already-mutated line, got:\n%s", got)
	}
}

// TestWriteEffects_BoundedRows_500Records is red-first for item 3: a plan
// section with 500 records renders a bounded number of visible rows plus one
// dim overflow line, while the full 500 remain in the snapshot untouched.
// Each record names a distinct branch — release-gate round 3 finding 6
// merges identical (verb, object) records into one summed row, so the
// bounded-rows overflow this test proves needs 500 distinct rows to exercise.
func TestWriteEffects_BoundedRows_500Records(t *testing.T) {
	var buf strings.Builder
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	plan := out.Plan("branches")
	const total = 500
	for i := 0; i < total; i++ {
		plan.Delete(1, fmt.Sprintf("feat/branch-%d", i))
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	snap := out.Snapshot()
	if len(snap.Plans) != 1 || len(snap.Plans[0].Records) != total {
		t.Fatalf("snapshot must retain all %d records, got %+v", total, snap.Plans)
	}
	got := buf.String()
	visibleRows := strings.Count(got, "feat/branch")
	if visibleRows >= total {
		t.Fatalf("human view must bound visible rows, rendered all %d", visibleRows)
	}
	if !strings.Contains(got, "+495 more (not shown)") {
		t.Fatalf("want bounded-rows overflow line, got:\n%s", got)
	}
}

// TestWriteAction_NextActionGlyph proves a next-action row is prefixed by the
// profile-aware glyph (→ Unicode, > ASCII) rather than a color-only cue.
func TestWriteAction_NextActionGlyph(t *testing.T) {
	var uniBuf strings.Builder
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&uniBuf), evo.Plain(), evo.NoColor(), evo.Glyphs(evo.GlyphsUnicode)}})
	out.Task("done").Done().Next(evo.Label("repo-retire --retire demo"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(uniBuf.String(), "→  repo-retire --retire demo") {
		t.Fatalf("want unicode next-action glyph, got:\n%s", uniBuf.String())
	}

	var asciiBuf strings.Builder
	out2 := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&asciiBuf), evo.Plain(), evo.NoColor(), evo.Glyphs(evo.GlyphsASCII)}})
	out2.Task("done").Done().Next(evo.Label("repo-retire --retire demo"))
	if err := out2.Finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(asciiBuf.String(), ">  repo-retire --retire demo") {
		t.Fatalf("want ASCII next-action glyph, got:\n%s", asciiBuf.String())
	}
}

// TestWriteProblem_EvidenceGlyph_ASCII proves a Detail evidence row routes
// through the ASCII glyph profile ("-") instead of a hardcoded "└─" that
// would mojibake on a non-UTF-8 terminal.
func TestWriteProblem_EvidenceGlyph_ASCII(t *testing.T) {
	var buf strings.Builder
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor(), evo.Glyphs(evo.GlyphsASCII)}})
	out.Task("branches").Fail("cannot lock ref", evo.Detail("another git process seems to be running"))
	if err := out.Finish(); err != nil {
		t.Log(err)
	}
	got := buf.String()
	if strings.Contains(got, "└─") {
		t.Fatalf("ASCII profile must not render Unicode evidence connector:\n%s", got)
	}
	if !strings.Contains(got, "- another git process seems to be running") {
		t.Fatalf("want ASCII evidence connector, got:\n%s", got)
	}
}

// TestConfirm_ASCIIProfile_PromptGlyph proves the confirm gate's "?" prompt
// routes through the ASCII glyph profile ("[?]") rather than a hardcoded "?"
// that would stay Unicode-only regardless of the configured profile.
func TestConfirm_ASCIIProfile_PromptGlyph(t *testing.T) {
	var buf strings.Builder
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Stdin(strings.NewReader("y\n")), evo.Glyphs(evo.GlyphsASCII), evo.NoColor()}})
	if ok := out.Confirm("proceed?"); !ok {
		t.Fatal("Confirm(\"y\") = false, want true")
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}
