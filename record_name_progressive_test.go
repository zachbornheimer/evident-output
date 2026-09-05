package evo_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// Red-first for the fix: a dry-run caller working through several tasks in
// sequence must see each task's RecordName items the moment that task's own
// work resolves — not only after every other task in the run reaches
// Output.Finish (evo-rec.md: "a --dry user loses 'what would run' per item"
// migrating go-task). Record/mutation-verb quantity tallies are unaffected
// (see TestWriteEffects_BoundedRows_500Records, TestCoalesce_*).

// TestRecordName_StreamsAtTaskResolution_PlainProfile proves the planned row
// is already durable right after the owning task resolves, before Finish
// ever runs — and that Finish does not also re-render it (no double
// "[planned]" band).
func TestRecordName_StreamsAtTaskResolution_PlainProfile(t *testing.T) {
	var buf bytes.Buffer
	// Config.Subject (like TestCoalesce_DryRunPlannedWithHeader_
	// SuppressesTrailingConclusion) is what lets a pure-planned multi-section
	// dry run suppress its trailing conclusion band, so the only "[planned]"
	// occurrences left to count are the two per-task ledger rows this test is
	// actually about.
	out := evo.Init(evo.Config{
		Isolated: true,
		DryRun:   true,
		Subject:  "repo  /demo",
		Options:  []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()},
	})
	t.Cleanup(func() { _ = out.Close() })

	build := out.Task("go-task")
	build.RecordName("run", "go build ./...")
	build.Done()

	// Before Finish ever runs: the per-item planned row must already be on
	// the stream, under this task's own block.
	before := buf.String()
	if !strings.Contains(before, "go build ./...") {
		t.Fatalf("want the planned row streamed at task resolution, before Finish; got:\n%q", before)
	}

	// A later task's own work must not race above the already-streamed row
	// (same chronology contract progressive Task rows already carry).
	other := out.Task("go-vet")
	other.RecordName("run", "go vet ./...")
	other.Done()

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if idx1, idx2 := strings.Index(got, "go build ./..."), strings.Index(got, "go vet ./..."); idx1 < 0 || idx2 < 0 || idx1 > idx2 {
		t.Fatalf("want go-task's row before go-vet's row (resolution order), got:\n%s", got)
	}
	if strings.Count(got, "[planned]") != 2 {
		t.Fatalf("want exactly one [planned] band per task (no Finish re-render), got %d in:\n%s",
			strings.Count(got, "[planned]"), got)
	}
}

// TestRecordName_StreamsAtTaskResolution_InteractiveProfile mirrors the
// plain-profile case for a live terminal driver: the durable row lands on
// the screen's scrollback (WriteDurable) the instant the task resolves, not
// only in WriteFinal's end-of-run tail.
func TestRecordName_StreamsAtTaskResolution_InteractiveProfile(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	// Title matches the task's own name so the single-matching-item
	// conclusion-suppression rule (TestCoalesce_SingleMatchingPlan) applies —
	// the only "[planned]" left to count is this task's own streamed row.
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.Title("go-task"),
		evo.Terminal(screen),
		evo.DryRun(),
	}})
	t.Cleanup(func() { _ = out.Close() })

	build := out.Task("go-task")
	build.RecordName("run", "go build ./...")
	build.Done()

	if !strings.Contains(screen.PersistedText(), "go build ./...") {
		t.Fatalf("want the planned row durably on screen at task resolution, got ops:\n%+v", screen.Operations())
	}

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Count(screen.PersistedText(), "[planned]") != 1 {
		t.Fatalf("Finish must not re-render the already-streamed row, got:\n%s", screen.PersistedText())
	}
}

// TestRecordName_CapsPerTaskRowsWithExactOverflow proves the bounded-rows
// cap + "+N more" overflow applies at the task-resolution streaming instant
// too — mirroring TestWriteEffects_BoundedRows_500Records's Finish-time
// bound, since the row cap is one shared ledger bound wherever it renders.
func TestRecordName_CapsPerTaskRowsWithExactOverflow(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DryRun()}})
	t.Cleanup(func() { _ = out.Close() })

	branches := out.Task("branches")
	const total = 500
	for i := 0; i < total; i++ {
		branches.RecordName("delete", fmt.Sprintf("feat/branch-%d", i))
	}
	branches.Done()

	// Streamed before Finish: cap + exact overflow already on the wire.
	before := buf.String()
	if !strings.Contains(before, "+495 more (not shown)") {
		t.Fatalf("want exact bounded-rows overflow streamed at task resolution, got:\n%s", before)
	}
	if visible := strings.Count(before, "feat/branch"); visible >= total {
		t.Fatalf("want bounded visible rows before Finish, rendered all %d", visible)
	}

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Count(buf.String(), "feat/branch") != strings.Count(before, "feat/branch") {
		t.Fatalf("Finish must not re-render the already-streamed named rows, got:\n%s", buf.String())
	}
}
