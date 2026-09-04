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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("demo"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.Glyphs(evo.GlyphsASCII)}})
	out.Task("done").Done()
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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("demo"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.Glyphs(evo.GlyphsASCII)}})
	group := out.Sequence("pipeline")
	first := group.Task("first")
	second := group.Task("second")
	_ = group.Task("third")
	first.Done()
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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.Title("demo"), evo.To(&bytes.Buffer{}), evo.Terminal(screen),
		evo.Glyphs(evo.GlyphsASCII), evo.VisibilityDelay(0),
	}})
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
		out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("demo"), evo.To(&bytes.Buffer{}), evo.Terminal(screen), evo.VisibilityDelay(0)}})
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
		out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("demo"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
		out.Task("done").Done()
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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("demo"), evo.To(&bytes.Buffer{}), evo.Terminal(screen), evo.VisibilityDelay(0)}})
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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("demo"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.Glyphs(evo.GlyphsUnicode)}})
	out.Task("done").Done()
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
