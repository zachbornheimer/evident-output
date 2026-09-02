package evo

import (
	"os"
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/internal/width"
)

// GlyphProfile selects which glyph vocabulary state markers render in
// (evo-rec.md "Tightened glyph vocabulary", rule GLYPH-001: glyph selection
// via capability profile, cell-width measurement not rune counts).
//
// The zero value, GlyphsAuto, keeps today's Unicode vocabulary off a TTY (a
// non-interactive stream can't show a human mojibake, so there is nothing to
// guard against) and on any TTY whose locale already advertises UTF-8. It
// downgrades to the ASCII vocabulary only on an interactive terminal without
// UTF-8 locale support — the one case where the status column would
// otherwise render as mojibake.
type GlyphProfile int

const (
	// GlyphsAuto detects the vocabulary from locale and TTY interactivity.
	GlyphsAuto GlyphProfile = iota
	// GlyphsUnicode forces the Unicode vocabulary regardless of locale.
	GlyphsUnicode
	// GlyphsASCII forces the ASCII vocabulary regardless of locale.
	GlyphsASCII
)

// Glyphs selects the glyph capability profile (default GlyphsAuto).
func Glyphs(p GlyphProfile) Option {
	return optionFunc(func(c *config) { c.glyphs = p })
}

// resolveGlyphProfileLocked turns a possibly-auto profile into a concrete
// one. Called once at construction (newOutput) so every render call reads a
// decided value instead of re-detecting locale per frame.
func resolveGlyphProfileLocked(cfg *config) {
	if cfg.glyphs != GlyphsAuto {
		return
	}
	if !interactiveOutputLocked(cfg) || localeAdvertisesUTF8() {
		cfg.glyphs = GlyphsUnicode
		return
	}
	cfg.glyphs = GlyphsASCII
}

// interactiveOutputLocked reports whether the configured terminal is a real
// interactive surface. Plain/non-interactive projection keeps the Unicode
// vocabulary unconditionally — evo-rec.md's glyph-safety default only
// guards the live status column a human is watching.
func interactiveOutputLocked(cfg *config) bool {
	if cfg.nonInteractive || cfg.plain {
		return false
	}
	ls := asLive(cfg.terminal)
	return ls != nil && ls.IsInteractive()
}

// localeAdvertisesUTF8 checks LC_ALL, then LC_CTYPE, then LANG (POSIX
// override order) for a UTF-8 marker. Read directly at construction time —
// the same pattern Config.Color's NO_COLOR detection uses — so no render
// path re-reads the environment.
func localeAdvertisesUTF8() bool {
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := os.Getenv(name); v != "" {
			return strings.Contains(v, "UTF-8") || strings.Contains(v, "utf8")
		}
	}
	// No locale env set at all: assume the historical default (UTF-8) rather
	// than guessing ASCII from absence.
	return true
}

// glyphSpec pairs one state's Unicode face with its 1:1 ASCII counterpart.
// evo-rec.md's tightened vocabulary keeps one meaning per symbol so a caller
// never has to guess which ASCII token stands in for which Unicode glyph.
type glyphSpec struct {
	unicode string
	ascii   string
}

// render returns the face for the given profile (GlyphsAuto is resolved
// before render time; treat it as GlyphsUnicode if it reaches here).
func (g glyphSpec) render(profile GlyphProfile) string {
	if profile == GlyphsASCII {
		return g.ascii
	}
	return g.unicode
}

// cellWidth reports the terminal cell width of the active face — measured in
// cells (internal/width), never rune or byte counts, per GLYPH-001.
func (g glyphSpec) cellWidth(profile GlyphProfile) int {
	return width.Cells(g.render(profile))
}

// State glyph table (evo-rec.md "Tightened glyph vocabulary"). Meanings not
// covered by the table (Running) are driven by the spinner alphabet instead
// of a static glyph.
var (
	glyphDone         = glyphSpec{"✓", "[ok]"}
	glyphFailedState  = glyphSpec{"✗", "[x]"}
	glyphBlockedState = glyphSpec{"⊘", "[blocked]"}
	glyphWarningState = glyphSpec{"!", "[!]"}
	glyphCancelled    = glyphSpec{"■", "[cancel]"}
	glyphNotStarted   = glyphSpec{"-", "[-]"}
	glyphPending      = glyphSpec{"○", "[.]"}
	glyphHumanInput   = glyphSpec{"?", "[?]"}
	// glyphUnclassified covers states with no distinct row in the vocabulary
	// table (e.g. Empty); it must stay visually distinct from Pending's "○".
	glyphUnclassified = glyphSpec{"·", "."}
)

// spinnerUnicodeFrames is the braille spinner sequence (common CLI convention).
var spinnerUnicodeFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinnerASCIIFrames excludes "-" (reserved for Not-started) per evo-rec.md's
// "ASCII spinner alphabet excludes every semantic glyph" rule.
var spinnerASCIIFrames = []string{".", "o", "O", "@"}

func spinnerFrames(profile GlyphProfile) []string {
	if profile == GlyphsASCII {
		return spinnerASCIIFrames
	}
	return spinnerUnicodeFrames
}

// spinnerGlyph picks a frame from the clock so FixedClock freezes it in tests.
func spinnerGlyph(now time.Time, profile GlyphProfile) string {
	frames := spinnerFrames(profile)
	if len(frames) == 0 {
		return glyphUnclassified.render(profile)
	}
	ns := now.UnixNano()
	if ns < 0 {
		ns = -ns
	}
	i := int(ns/int64(spinnerPeriod)) % len(frames)
	return frames[i]
}

// itemGlyph maps an Item's state to its glyph in the given profile. The
// state→meaning mapping is unchanged from before the profile axis existed;
// only the rendered face (Unicode vs ASCII) varies.
func itemGlyph(s EntityState, profile GlyphProfile) string {
	switch s {
	case OK:
		return glyphDone.render(profile)
	case Failed:
		return glyphFailedState.render(profile)
	case Blocked:
		return glyphBlockedState.render(profile)
	case Warning:
		return glyphWarningState.render(profile)
	case Skipped:
		return glyphPending.render(profile)
	case Unknown, Incomplete:
		return glyphHumanInput.render(profile)
	case Running:
		return spinnerFrames(profile)[0]
	case Cancelled:
		return glyphCancelled.render(profile)
	default:
		return glyphUnclassified.render(profile)
	}
}

// taskGlyph maps a Task's state to its glyph in the given profile. Cancelled
// and Skipped share Pending's glyph, matching the vocabulary this table
// replaced — that mapping is preserved verbatim, not revisited here.
func taskGlyph(s EntityState, profile GlyphProfile) string {
	switch s {
	case Done:
		return glyphDone.render(profile)
	case Failed:
		return glyphFailedState.render(profile)
	case Blocked:
		return glyphBlockedState.render(profile)
	case Warning:
		return glyphWarningState.render(profile)
	case Running:
		return spinnerFrames(profile)[0]
	case Pending:
		return glyphPending.render(profile)
	case Cancelled, Skipped:
		return glyphPending.render(profile)
	case NotStarted:
		return glyphNotStarted.render(profile)
	default:
		return glyphUnclassified.render(profile)
	}
}
