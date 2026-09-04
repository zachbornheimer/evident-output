package evo

import (
	"os"
	"strings"
	"time"

	txt "github.com/zachbornheimer/evident-output/internal/text"
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
	if cfg.plain {
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
// cells (internal/text), never rune or byte counts, per GLYPH-001.
func (g glyphSpec) cellWidth(profile GlyphProfile) int {
	return txt.Cells(g.render(profile))
}

// State glyph table (evo-rec.md "Tightened glyph vocabulary"). A live
// interactive redraw drives Running from the spinner alphabet instead
// (writeLiveTaskLine overrides taskGlyph's Running result with the current
// animated frame); every other consumer — a plain/non-interactive durable
// line, a residual dump, a snapshot — has no animation loop behind it, so
// taskGlyph gives Running its own static face (glyphRunning) rather than a
// spinner frame frozen mid-spin (beginner-8: "no spinner glyph in plain").
var (
	glyphDone         = glyphSpec{"✓", "[ok]"}
	glyphFailedState  = glyphSpec{"✗", "[x]"}
	glyphBlockedState = glyphSpec{"⊘", "[blocked]"}
	glyphWarningState = glyphSpec{"!", "[!]"}
	glyphCancelled    = glyphSpec{"■", "[cancel]"}
	glyphNotStarted   = glyphSpec{"-", "[-]"}
	glyphPending      = glyphSpec{"○", "[.]"}
	glyphRunning      = glyphSpec{"◐", "[~]"}
	glyphHumanInput   = glyphSpec{"?", "[?]"}
	// glyphNextAction marks a follow-up command/label line. evo-rec.md's
	// tightened vocabulary table gives it its own row so the meaning does not
	// depend on the cyan color alone (rule: text/glyph carries meaning).
	glyphNextAction = glyphSpec{"→", ">"}
	// glyphEvidence is the tree connector for Detail/Cause rows under a
	// Problem — dim per "Color and style demotions" (subordinate evidence).
	glyphEvidence = glyphSpec{"└─", "-"}
	// glyphOverflow marks a truncated/omitted-count line ("… +N more").
	glyphOverflow = glyphSpec{"…", "..."}
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

// taskGlyph maps a Task's state to its glyph in the given profile. Cancelled
// gets its own face (glyphCancelled) per the tightened vocabulary table —
// it must be visually distinct from a task that never got attention.
// Skipped keeps Pending's glyph, unchanged from before the table existed.
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
		return glyphRunning.render(profile)
	case Pending, Skipped:
		return glyphPending.render(profile)
	case Cancelled:
		return glyphCancelled.render(profile)
	// Incomplete: a task Finish left non-terminal (never ran or never
	// resolved) has no dedicated row in the vocabulary table; it reads as
	// "not started" rather than the unclassified "·" no state should ever
	// need (evo-rec.md "Conclusion algebra").
	case NotStarted, Incomplete:
		return glyphNotStarted.render(profile)
	default:
		return glyphUnclassified.render(profile)
	}
}
