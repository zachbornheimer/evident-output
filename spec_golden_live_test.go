package evo_test

import (
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// newLiveScreenOutput builds an Output over an interactive testkit.Screen
// with VisibilityDelay(0) (first paint is immediate) and MaxFrameRate raised
// far above the default 20fps. Without the raised frame rate, a second
// evidence call (Progress/Bytes) issued within the same wall-clock
// millisecond as the task's declare-time paint gets coalesced by the
// real-time min-gap guard in signalLiveLocked, and LatestLiveText() would
// return the stale declare-time frame instead of the frame under test.
// Phase() calls force a repaint regardless (used by Each internally), which
// is why simple single-call Phase-driven probes didn't need this — but a
// bare Progress/Bytes call does.
func newLiveScreenOutput(screen *testkit.Screen, opts ...evo.Option) *evo.Output {
	base := []evo.Option{
		evo.Terminal(screen),
		evo.VisibilityDelay(0),
		evo.MaxFrameRate(1_000_000),
		evo.NoColor(),
	}
	return evo.Init(evo.Config{Options: append(base, opts...)})
}

// TestSpecP1_LiveFrame_Step1 covers evo-rec.md Problem 1's step1 block via
// the spec's own taught idiom for iterating named items, TaskHandle.Each:
//
//	:.  branches  1/40  feat/old-billing
//
// MISMATCH (documented, not fixed): the real first frame is
// "<spinner>  branches  [░░░░░░░░░░░░]  0/40  feat/old-billing" — two
// differences from the 2026-vintage illustration above, both because Each's
// contract postdates it (see each.go): (1) Each drives absolute
// Progress(i, total) *before* yielding item i, so the first frame reads
// "items completed so far" (0) rather than "current position" (1) — this is
// each.go's documented, deliberately-adopted policy ("the bar reads 'items
// completed so far', not 'items completed including the one still
// running'"), not a bug; (2) any determinate Progress row always renders a
// 12-cell bar (writeLiveTaskLine, live.go:600-602), which live_progress_bar_test.go
// already pins as intentional for count progress, not just byte progress.
// Both are deliberately-adopted library behavior; the older illustration is
// SPEC-STALE relative to them. This test proves the real (correct) output,
// not the stale text.
func TestSpecP1_LiveFrame_Step1(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	names := make([]string, 40)
	for i := range names {
		names[i] = "feat/x" + string(rune('a'+i%26))
	}
	names[0] = "feat/old-billing"
	branches := out.Task("branches")
	for range branches.Each(names) {
		break
	}

	got := screen.LatestLiveText()
	for _, want := range []string{"branches", "0/40", "feat/old-billing", "[", "]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in live frame:\n%s", want, got)
		}
	}
}

// TestSpecP1_LiveFrame_Step2 covers evo-rec.md Problem 1's step2 block: one
// Running child (worktrees), a prior Done sibling that survives (branches),
// and a later sibling still named/idle (remotes) — driven as three
// independent top-level Tasks (not a Group/Tasks collection), which is why
// there is no collection header line, matching the spec block exactly:
//
//	✓  branches  14 deleted
//	:.  worktrees  1/3  ../.worktrees/app-sah-1
//	○  remotes
func TestSpecP1_LiveFrame_Step2(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("branches").Done("14 deleted")
	worktrees := out.Task("worktrees")
	out.Task("remotes") // declared, not yet started — renders "○  remotes"
	for range worktrees.Each([]string{"../.worktrees/app-sah-1", "../.worktrees/app-sah-2", "../.worktrees/app-sah-3"}) {
		break
	}

	got := screen.LatestLiveText()
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 flat task lines (no collection header), got %d:\n%s", len(lines), got)
	}
	if !strings.Contains(lines[0], "✓") || !strings.Contains(lines[0], "branches") || !strings.Contains(lines[0], "14 deleted") {
		t.Fatalf("line 1 want Done branches summary, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "worktrees") || !strings.Contains(lines[1], "../.worktrees/app-sah-1") {
		t.Fatalf("line 2 want Running worktrees with current name, got %q", lines[1])
	}
	if !strings.Contains(lines[2], "○") || !strings.Contains(lines[2], "remotes") {
		t.Fatalf("line 3 want pending remotes with ○, got %q", lines[2])
	}
}

// TestSpecP1_LiveFrame_Indeterminate covers evo-rec.md Problem 1's
// indeterminate block: an unsealed total renders a phase string, not a fake
// count.
//
//	:.  branches  classifying…
//	○  worktrees
func TestSpecP1_LiveFrame_Indeterminate(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("branches").Phase("classifying…")
	out.Task("worktrees")

	got := screen.LatestLiveText()
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 flat task lines, got %d:\n%s", len(lines), got)
	}
	if !strings.Contains(lines[0], "branches") || !strings.Contains(lines[0], "classifying…") {
		t.Fatalf("line 1 want indeterminate phase, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "○") || !strings.Contains(lines[1], "worktrees") {
		t.Fatalf("line 2 want pending ○, got %q", lines[1])
	}
}

// TestSpecP3_LiveFrame_Step2 covers evo-rec.md Problem 3's step2 block: once
// --dry-run is dropped, the same task now drives a live absolute Progress.
//
//	:.  salvage  1/3  feat/a
//
// Called directly with Progress+Phase (not Each) since this depicts a named
// current status mid-run, not a caller-owned name list — the literal count
// the spec shows is reachable exactly this way, with no 0-vs-1 divergence.
func TestSpecP3_LiveFrame_Step2(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	salvage := out.Task("salvage")
	salvage.Progress(1, 3)
	salvage.Phase("feat/a")

	got := screen.LatestLiveText()
	for _, want := range []string{"salvage", "1/3", "feat/a"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in live frame:\n%s", want, got)
		}
	}
}

// TestSpecP3_LiveFrame_Indeterminate covers evo-rec.md Problem 3's
// indeterminate block ("still planning, no progress total yet").
//
//	:.  salvage  planning…
func TestSpecP3_LiveFrame_Indeterminate(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("salvage").Phase("planning…")

	got := screen.LatestLiveText()
	if !strings.Contains(got, "salvage") || !strings.Contains(got, "planning…") {
		t.Fatalf("want indeterminate phase in live frame:\n%s", got)
	}
}

// TestSpecP4_LiveFrame_Step1 covers evo-rec.md Problem 4's step1 block: a
// sequential Group presents one Running child with siblings named idle.
//
//	python
//	:.  scan     scanning
//	   venv
//	   install
//
// MISMATCH (documented, not fixed): the real frame adds a collection header
// line ("<spinner>  python  0/3 complete") the illustration omits, and every
// pending sibling row carries an explicit "○" — the latter is *required* by
// the "Adopted revisions" table later in this same spec ("Pending rows
// always carry ○... Whitespace never represents state"), which supersedes
// this earlier blank-indent illustration. The header-count suffix has no
// counterpart rule either way; it is additional (not contradicting)
// information, left as-is.
func TestSpecP4_LiveFrame_Step1(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	setup := out.Group("python")
	scan := setup.Task("scan")
	setup.Task("venv")
	setup.Task("install")
	scan.Phase("scanning")

	got := screen.LatestLiveText()
	lines := strings.Split(got, "\n")
	if len(lines) != 4 {
		t.Fatalf("want header + 3 children, got %d:\n%s", len(lines), got)
	}
	if !strings.Contains(lines[0], "python") {
		t.Fatalf("want group name on header line, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "scan") || !strings.Contains(lines[1], "scanning") {
		t.Fatalf("want Running scan with phase, got %q", lines[1])
	}
	if !strings.Contains(lines[2], "○") || !strings.Contains(lines[2], "venv") {
		t.Fatalf("want pending venv with ○, got %q", lines[2])
	}
	if !strings.Contains(lines[3], "○") || !strings.Contains(lines[3], "install") {
		t.Fatalf("want pending install with ○, got %q", lines[3])
	}
	// One Running child heart-contract: exactly one non-pending child glyph.
	if strings.Count(got, "○") != 2 {
		t.Fatalf("want exactly 2 pending siblings, got:\n%s", got)
	}
}

// TestSpecP4_LiveFrame_Step2 covers Problem 4's step2 block: scan resolves,
// venv becomes the one Running child, install stays pending.
//
//	python
//	✓  scan
//	:.  venv     creating
//	   install
func TestSpecP4_LiveFrame_Step2(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	setup := out.Group("python")
	scan := setup.Task("scan")
	venv := setup.Task("venv")
	setup.Task("install")
	scan.Done()
	venv.Phase("creating")

	got := screen.LatestLiveText()
	if !strings.Contains(got, "✓") || !strings.Contains(got, "scan") {
		t.Fatalf("want Done scan, got:\n%s", got)
	}
	if !strings.Contains(got, "venv") || !strings.Contains(got, "creating") {
		t.Fatalf("want Running venv with phase, got:\n%s", got)
	}
	if !strings.Contains(got, "○") || !strings.Contains(got, "install") {
		t.Fatalf("want pending install, got:\n%s", got)
	}
}

// TestSpecP5_LiveFrame_IndeterminateThenSealed covers evo-rec.md Problem 5's
// step1/indeterminate block (unknown total, phase only) and step2 block (the
// total seals, absolute Progress takes over) in one run, proving the
// transition never resets or fakes an early total.
//
//	:.  scan  discovering…              (step1 / indeterminate)
//	:.  scan  14/128  ~/Developer/Personal/zq   (step2)
func TestSpecP5_LiveFrame_IndeterminateThenSealed(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	scan := out.Task("scan")
	scan.Phase("discovering…")
	before := screen.LatestLiveText()
	if !strings.Contains(before, "scan") || !strings.Contains(before, "discovering…") {
		t.Fatalf("want indeterminate discovery phase, got:\n%s", before)
	}

	scan.Progress(14, 128)
	scan.Phase("~/Developer/Personal/zq") // forces a repaint reflecting the now-sealed total
	after := screen.LatestLiveText()
	for _, want := range []string{"scan", "14/128", "~/Developer/Personal/zq"} {
		if !strings.Contains(after, want) {
			t.Fatalf("want %q in sealed-progress frame:\n%s", want, after)
		}
	}
}

// TestSpecP6_LiveFrame_BytesBar covers evo-rec.md Problem 6's step1 block:
// byte totals use Bytes (a progress bar + "completed/total MB"), never the
// item-count Progress API.
//
//	:.  generate  [██░░░░░░░░░░]  2.1/8.0 MB
func TestSpecP6_LiveFrame_BytesBar(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("generate").Bytes(2_100_000, 8_000_000)

	got := screen.LatestLiveText()
	for _, want := range []string{"generate", "[", "]", "2.1/8.0 MB"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in bytes live frame:\n%s", want, got)
		}
	}
}

// TestSpecP6_LiveFrame_Step2 covers Problem 6's step2 block: the byte task
// resolves, and a sibling count-based task takes over as the one Running
// child, with the exact literal count the spec shows (a direct Progress call
// for a named current status, not a caller-owned name list, has no 0-vs-1
// divergence — see TestSpecP1_LiveFrame_Step1's note).
//
//	✓  generate  8.0 MB
//	:.  test      [████░░░░░░░░]  4/12
func TestSpecP6_LiveFrame_Step2(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("generate").Done("8.0 MB")
	test := out.Task("test")
	test.Progress(4, 12)
	test.Phase("") // forces a repaint reflecting the just-set Progress

	got := screen.LatestLiveText()
	if !strings.Contains(got, "✓") || !strings.Contains(got, "generate") || !strings.Contains(got, "8.0 MB") {
		t.Fatalf("want Done generate, got:\n%s", got)
	}
	if !strings.Contains(got, "test") || !strings.Contains(got, "4/12") {
		t.Fatalf("want Running test at 4/12, got:\n%s", got)
	}
}

// TestSpecP7_LiveFrame_Step2 covers evo-rec.md Problem 7's step2 block: a
// viewport-truncated task (500 records planned) still renders one honest
// live progress row keyed on the current name, at the literal count the
// spec shows.
//
//	:.  branches  40/500  feat/zz-old
func TestSpecP7_LiveFrame_Step2(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	branches := out.Task("branches")
	branches.Progress(40, 500)
	branches.Phase("feat/zz-old")

	got := screen.LatestLiveText()
	for _, want := range []string{"branches", "40/500", "feat/zz-old"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in live frame:\n%s", want, got)
		}
	}
}

// TestSpecP8_LiveFrame_Step1 covers evo-rec.md Problem 8's step1 block:
// deleting remote refs one at a time, at the literal count the spec shows.
//
//	:.  remotes  1/3  origin/feat/a
func TestSpecP8_LiveFrame_Step1(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	remotes := out.Task("remotes")
	remotes.Progress(1, 3)
	remotes.Phase("origin/feat/a")

	got := screen.LatestLiveText()
	for _, want := range []string{"remotes", "1/3", "origin/feat/a"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in live frame:\n%s", want, got)
		}
	}
}

// TestSpecP8_LiveFrame_Step2 covers Problem 8's step2 block: the prior
// delete survives as a Done row while the next one becomes the one Running
// child.
//
//	✓  remotes  deleted origin/feat/a
//	:.  remotes  2/3  origin/feat/b
//
// NOT-TESTABLE as a single frame through the public API as spelled: both
// rows share the name "remotes", but a Task is a single resolvable entity —
// one TaskHandle cannot be simultaneously Done (for feat/a) and Running (for
// feat/b) in the same snapshot. The spec's two-row shape needs either two
// distinct Task handles (which would print two different names, not both
// "remotes") or a single evolving row per unit of work (which the rest of
// this problem's blocks already show, e.g. step1 above) — reaching for the
// simplest documented spelling cannot reproduce two rows under one name.
func TestSpecP8_LiveFrame_Step2_NotTestable(t *testing.T) {
	t.Skip("NOT-TESTABLE: see doc comment — a single TaskHandle cannot render two rows (Done + Running) under one name in one frame")
}

// TestSpecP9_LiveFrame_Step1 covers evo-rec.md Problem 9's step1 block (this
// text is identical to the spec's "indeterminate" block for the same
// problem, so this test's verdict covers both):
//
//	✓  scan
//	:.  venv  creating
func TestSpecP9_LiveFrame_Step1(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("scan").Done()
	out.Task("venv").Phase("creating")

	got := screen.LatestLiveText()
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 flat task lines, got %d:\n%s", len(lines), got)
	}
	if !strings.Contains(lines[0], "✓") || !strings.Contains(lines[0], "scan") {
		t.Fatalf("line 1 want Done scan, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "venv") || !strings.Contains(lines[1], "creating") {
		t.Fatalf("line 2 want Running venv with phase, got %q", lines[1])
	}
}

// TestSpecP9_LiveFrame_Step2 covers Problem 9's step2 block: a third,
// not-yet-started sibling is now declared and renders idle.
//
//	✓  scan
//	:.  venv  creating
//	○  install
func TestSpecP9_LiveFrame_Step2(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("scan").Done()
	out.Task("venv").Phase("creating")
	out.Task("install")

	got := screen.LatestLiveText()
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 flat task lines, got %d:\n%s", len(lines), got)
	}
	if !strings.Contains(lines[2], "○") || !strings.Contains(lines[2], "install") {
		t.Fatalf("line 3 want pending install, got %q", lines[2])
	}
}

// TestSpecP11_LiveFrame_Step1 covers evo-rec.md Problem 11's step1 block: a
// named parent group ("pipeline") with children carrying Progress, driven
// via evo.Group exactly like Problem 4's already-proven pattern.
//
//	pipeline
//	:.  go mod download  1/4
//	   go generate
//	   go test ./...
func TestSpecP11_LiveFrame_Step1(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	pipeline := out.Group("pipeline")
	download := pipeline.Task("go mod download")
	pipeline.Task("go generate")
	pipeline.Task("go test ./...")
	download.Progress(1, 4)

	got := screen.LatestLiveText()
	if !strings.Contains(got, "pipeline") {
		t.Fatalf("want group name on header line, got:\n%s", got)
	}
	if !strings.Contains(got, "go mod download") || !strings.Contains(got, "1/4") {
		t.Fatalf("want Running download child at 1/4, got:\n%s", got)
	}
	if !strings.Contains(got, "○") || !strings.Contains(got, "go generate") || !strings.Contains(got, "go test ./...") {
		t.Fatalf("want two pending siblings, got:\n%s", got)
	}
}

// TestSpecP25_LiveFrame_ASCIISpinner covers evo-rec.md Problem 25's step1
// block: the ASCII spinner alphabet excludes every semantic glyph (GLYPH-001
// / "the ASCII spinner alphabet excludes every semantic glyph so no frame
// collides with '-' (not started)").
//
//	\  branches  1/40  feat/old-billing
func TestSpecP25_LiveFrame_ASCIISpinner(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen, evo.Glyphs(evo.GlyphsASCII))
	t.Cleanup(func() { _ = out.Close() })

	branches := out.Task("branches")
	branches.Progress(1, 40)
	branches.Phase("feat/old-billing")

	got := screen.LatestLiveText()
	if !strings.Contains(got, "branches") || !strings.Contains(got, "1/40") || !strings.Contains(got, "feat/old-billing") {
		t.Fatalf("want branches row with count and current name, got:\n%s", got)
	}
	if strings.ContainsAny(got, "✓✗⊘■○→…") {
		t.Fatalf("ASCII profile must not leak a Unicode glyph, got:\n%s", got)
	}
	glyphColumn := strings.Fields(got)[0]
	if strings.Contains(glyphColumn, "-") {
		t.Fatalf("spinner glyph must never collide with the not-started glyph '-', got %q", glyphColumn)
	}
}

// TestSpecP16_LiveFrame_NarrowTerminal_CompactDialect covers evo-rec.md
// Problem 16/26's compact dialect for a determinate-progress live row below
// compactLayoutMaxWidth (40 cols): "narrow terminals degrade by dropping
// decoration (the bar) before information (count, name)" —
//
//	:. branches 3/40 ft/old…
//
// FIXED (was TestSpecP16_LiveFrame_NarrowTerminal_Mismatch): writeLiveTaskLine
// (live.go) now carries a width-aware compact branch for the
// determinate-progress row, mirroring writeEffects's compactLayoutMaxWidth
// check — the fixed 12-cell bar is dropped below the threshold so
// fitLiveRegion's whole-line truncation has only decoration left to eat into,
// not the count or the current name.
func TestSpecP16_LiveFrame_NarrowTerminal_CompactDialect(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(30), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	branches := out.Task("branches")
	branches.Progress(3, 40)
	branches.Phase("feat/old-billing-migration")

	got := screen.LatestLiveText()
	for _, want := range []string{"3/40", "branches"} {
		if !strings.Contains(got, want) {
			t.Fatalf("compact dialect must keep %q, got %q", want, got)
		}
	}
	if strings.Contains(got, "[") || strings.Contains(got, "█") || strings.Contains(got, "░") {
		t.Fatalf("compact dialect must drop the progress bar below compactLayoutMaxWidth, got %q", got)
	}
}

// TestSpecP26_LiveFrame_ResizeMidRun_DropsToCompactDialect covers evo-rec.md
// Problem 26 step1: the terminal is narrowed mid-run, and the very next
// frame must recompute the layout live rather than leave stale wrapped
// residue — "a resize is a rerender, never residue". A wide first frame
// keeps the bar; after testkit.Screen.SetSize narrows the pane below
// compactLayoutMaxWidth, the following frame drops to the compact dialect
// (count + name survive, bar gone) in the same render call, not on some
// later frame.
func TestSpecP26_LiveFrame_ResizeMidRun_DropsToCompactDialect(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	branches := out.Task("branches")
	branches.Progress(3, 40)
	branches.Phase("feat/old-billing-migration")

	wide := screen.LatestLiveText()
	if !strings.Contains(wide, "[") {
		t.Fatalf("wide frame should keep the progress bar, got %q", wide)
	}

	screen.SetSize(30, 0)
	branches.Progress(4, 40)

	narrow := screen.LatestLiveText()
	for _, want := range []string{"4/40", "branches"} {
		if !strings.Contains(narrow, want) {
			t.Fatalf("post-resize compact dialect must keep %q, got %q", want, narrow)
		}
	}
	if strings.Contains(narrow, "[") || strings.Contains(narrow, "█") || strings.Contains(narrow, "░") {
		t.Fatalf("post-resize frame must drop the bar, not leave wrapped residue, got %q", narrow)
	}
}
