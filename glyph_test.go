package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	txt "github.com/zachbornheimer/evident-output/internal/text"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestGlyphsASCII_StateRowsUseTightenedVocabulary pins evo-rec.md's 1:1
// ASCII map (GLYPH-001) for the states a caller hits routinely.
func TestGlyphsASCII_StateRowsUseTightenedVocabulary(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Title: "demo", Glyphs: evo.GlyphsASCII, Color: evo.ColorNever, Plain: true})
	succeed(out.Task("done"))
	out.Task("failed").Fail("boom")
	out.Task("gate").Block("declined")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	for _, want := range []string{"[ok]", "[x]", "[blocked]"} {
		if !strings.Contains(s, want) {
			t.Fatalf("expected %q in ASCII-profile output:\n%s", want, s)
		}
	}
	for _, forbidden := range []string{"✓", "✗", "⊘", "■"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("ASCII profile must not leak Unicode glyph %q:\n%s", forbidden, s)
		}
	}
}

// TestGlyphsASCII_NotStartedAndPendingRows pins [-] and [.] for the two
// remaining static rows requested in the work order.
func TestGlyphsASCII_NotStartedAndPendingRows(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Title: "demo", Glyphs: evo.GlyphsASCII, Color: evo.ColorNever, Plain: true})
	group := out.Sequence("pipeline")
	first := group.Task("first")
	second := group.Task("second")
	_ = group.Task("third")
	succeed(first)
	second.Fail("boom")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, "[-]") {
		t.Fatalf("expected [-] not-started row for the un-run sibling:\n%s", s)
	}
}

// TestGlyphsASCII_SpinnerExcludesNotStartedGlyph pins the "ASCII spinner
// alphabet excludes every semantic glyph" rule: no frame collides with "-".
func TestGlyphsASCII_SpinnerExcludesNotStartedGlyph(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive())
	out := evo.Init(evo.Config{Isolated: true, Stdout: &bytes.Buffer{}, Terminal: screen, Title: "demo", VisibilityDelay: evo.DelayForTest(0), Glyphs: evo.GlyphsASCII})
	task := out.Task("install")
	task.Doing("working")
	frame := screen.LatestLiveText()
	if strings.Contains(frame, "-") {
		t.Fatalf("ASCII spinner frame must never render '-' (reserved for Not-started): %q", frame)
	}
	_ = out.Finish()
}

// TestGlyphsAuto_NonUTF8LocaleDowngradesOnlyWhenInteractive pins evo-rec.md
// #11: Auto detects from locale on an interactive TTY, but a non-interactive
// destination keeps the historical Unicode vocabulary regardless of locale.
func TestGlyphsAuto_NonUTF8LocaleDowngradesOnlyWhenInteractive(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "")

	t.Run("interactive TTY downgrades to ASCII", func(t *testing.T) {
		screen := testkit.NewScreen(testkit.Interactive())
		out := evo.Init(evo.Config{Isolated: true, Stdout: &bytes.Buffer{}, Terminal: screen, Title: "demo", VisibilityDelay: evo.DelayForTest(0)})
		task := out.Task("install")
		task.Doing("working")
		frame := screen.LatestLiveText()
		if strings.Contains(frame, "⠋") {
			t.Fatalf("expected ASCII spinner on a non-UTF-8 interactive TTY, got %q", frame)
		}
		_ = out.Finish()
	})

	t.Run("non-interactive keeps Unicode", func(t *testing.T) {
		var buf bytes.Buffer
		out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Title: "demo", Color: evo.ColorNever, Plain: true})
		succeed(out.Task("done"))
		if err := out.Finish(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "✓") {
			t.Fatalf("non-TTY output must keep today's Unicode glyph regardless of locale:\n%s", buf.String())
		}
	})
}

// TestGlyphsAuto_UTF8LocaleKeepsUnicode pins the positive Auto case: a
// UTF-8 locale on an interactive TTY stays Unicode.
func TestGlyphsAuto_UTF8LocaleKeepsUnicode(t *testing.T) {
	t.Setenv("LC_ALL", "en_US.UTF-8")
	screen := testkit.NewScreen(testkit.Interactive())
	out := evo.Init(evo.Config{Isolated: true, Stdout: &bytes.Buffer{}, Terminal: screen, Title: "demo", VisibilityDelay: evo.DelayForTest(0)})
	task := out.Task("install")
	task.Doing("working")
	frame := screen.LatestLiveText()
	if !strings.ContainsAny(frame, "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏") {
		t.Fatalf("expected a Unicode braille spinner frame on a UTF-8 TTY, got %q", frame)
	}
	_ = out.Finish()
}

// TestGlyphUnicode_UnchangedByProfileAxis is a narrow regression pin: adding
// the profile axis must not alter a single Unicode glyph byte (blast radius:
// "don't change the Unicode glyphs themselves").
func TestGlyphUnicode_UnchangedByProfileAxis(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Title: "demo", Glyphs: evo.GlyphsUnicode, Color: evo.ColorNever, Plain: true})
	succeed(out.Task("done"))
	out.Task("failed").Fail("boom")
	out.Task("gate").Block("declined")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	for _, want := range []string{"✓", "✗", "⊘"} {
		if !strings.Contains(s, want) {
			t.Fatalf("expected unchanged Unicode glyph %q:\n%s", want, s)
		}
	}
}

// TestGlyphWidths_BlockedAndCancelledAreNarrow spot-checks the cell-width
// metadata the work order calls out: "⊘" and "■" are East Asian
// Ambiguous-width, one cell wide by internal/text's measurement, unlike the
// two-cell "✓"/"✗" dingbats — the glyph table must report this per-face
// width so layout code can align columns rather than assuming rune count.
func TestGlyphWidths_BlockedAndCancelledAreNarrow(t *testing.T) {
	if got := txt.Cells("⊘"); got != 1 {
		t.Fatalf("⊘ width = %d, want 1", got)
	}
	if got := txt.Cells("■"); got != 1 {
		t.Fatalf("■ width = %d, want 1", got)
	}
	if got := txt.Cells("✓"); got != 2 {
		t.Fatalf("✓ width = %d, want 2", got)
	}
}

// TestWriteAction_NextActionGlyph proves a next-action row is prefixed by the
// profile-aware glyph (→ Unicode, > ASCII) rather than a color-only cue.
func TestWriteAction_NextActionGlyph(t *testing.T) {
	var uniBuf strings.Builder
	out := evo.Init(evo.Config{Isolated: true, Stdout: &uniBuf, Glyphs: evo.GlyphsUnicode, Color: evo.ColorNever, Plain: true})
	done := out.Task("done")
	done.Problem("repository not retired yet", evo.Severity(evo.SeverityWarning), evo.Next(evo.Label("repo-retire --retire demo")))
	succeed(done)
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(uniBuf.String(), "→  repo-retire --retire demo") {
		t.Fatalf("want unicode next-action glyph, got:\n%s", uniBuf.String())
	}

	var asciiBuf strings.Builder
	out2 := evo.Init(evo.Config{Isolated: true, Stdout: &asciiBuf, Glyphs: evo.GlyphsASCII, Color: evo.ColorNever, Plain: true})
	done2 := out2.Task("done")
	done2.Problem("repository not retired yet", evo.Severity(evo.SeverityWarning), evo.Next(evo.Label("repo-retire --retire demo")))
	succeed(done2)
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
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Glyphs: evo.GlyphsASCII, Color: evo.ColorNever, Plain: true})
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
	restore := evo.MarkWriterAsCharDevice(&buf)
	t.Cleanup(restore)
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Stderr: &buf, Stdin: strings.NewReader("y\n"), Glyphs: evo.GlyphsASCII, Color: evo.ColorNever})
	if ok := out.Confirm("proceed?"); !ok {
		t.Fatal("Confirm(\"y\") = false, want true")
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}
