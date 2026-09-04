package text

import "time"

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

// glyphSpec pairs one state's Unicode face with its 1:1 ASCII counterpart.
// evo-rec.md's tightened vocabulary keeps one meaning per symbol so a caller
// never has to guess which ASCII token stands in for which Unicode glyph.
type glyphSpec struct {
	unicode string
	ascii   string
}

// Render returns the face for the given profile (GlyphsAuto is resolved
// before render time; treat it as GlyphsUnicode if it reaches here).
func (g glyphSpec) Render(profile GlyphProfile) string {
	if profile == GlyphsASCII {
		return g.ascii
	}
	return g.unicode
}

// CellWidth reports the terminal cell width of the active face — measured in
// cells, never rune or byte counts, per GLYPH-001.
func (g glyphSpec) CellWidth(profile GlyphProfile) int {
	return Cells(g.Render(profile))
}

// State glyph table (evo-rec.md "Tightened glyph vocabulary"). A live
// interactive redraw drives Running from the spinner alphabet instead
// (writeLiveTaskLine overrides TaskGlyph's Running result with the current
// animated frame); every other consumer — a plain/non-interactive durable
// line, a residual dump, a snapshot — has no animation loop behind it, so
// TaskGlyph gives Running its own static face (GlyphRunning) rather than a
// spinner frame frozen mid-spin (beginner-8: "no spinner glyph in plain").
var (
	GlyphDone         = glyphSpec{"✓", "[ok]"}
	GlyphFailedState  = glyphSpec{"✗", "[x]"}
	GlyphBlockedState = glyphSpec{"⊘", "[blocked]"}
	GlyphWarningState = glyphSpec{"!", "[!]"}
	GlyphCancelled    = glyphSpec{"■", "[cancel]"}
	GlyphNotStarted   = glyphSpec{"-", "[-]"}
	GlyphPending      = glyphSpec{"○", "[.]"}
	GlyphRunning      = glyphSpec{"◐", "[~]"}
	GlyphHumanInput   = glyphSpec{"?", "[?]"}
	// GlyphNextAction marks a follow-up command/label line. evo-rec.md's
	// tightened vocabulary table gives it its own row so the meaning does not
	// depend on the cyan color alone (rule: text/glyph carries meaning).
	GlyphNextAction = glyphSpec{"→", ">"}
	// GlyphEvidence is the tree connector for Detail/Cause rows under a
	// Problem — dim per "Color and style demotions" (subordinate evidence).
	GlyphEvidence = glyphSpec{"└─", "-"}
	// GlyphOverflow marks a truncated/omitted-count line ("… +N more").
	GlyphOverflow = glyphSpec{"…", "..."}
	// GlyphUnclassified covers states with no distinct row in the vocabulary
	// table (e.g. Empty); it must stay visually distinct from Pending's "○".
	GlyphUnclassified = glyphSpec{"·", "."}
)

// SpinnerPeriod is the wall-clock duration between spinner frame advances.
const SpinnerPeriod = 80 * time.Millisecond

// spinnerUnicodeFrames is the braille spinner sequence (common CLI convention).
var spinnerUnicodeFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinnerASCIIFrames excludes "-" (reserved for Not-started) per evo-rec.md's
// "ASCII spinner alphabet excludes every semantic glyph" rule.
var spinnerASCIIFrames = []string{".", "o", "O", "@"}

// SpinnerFrames returns the animation alphabet for profile.
func SpinnerFrames(profile GlyphProfile) []string {
	if profile == GlyphsASCII {
		return spinnerASCIIFrames
	}
	return spinnerUnicodeFrames
}

// SpinnerGlyph picks a frame from the clock so a fixed clock freezes it in tests.
func SpinnerGlyph(now time.Time, profile GlyphProfile) string {
	frames := SpinnerFrames(profile)
	if len(frames) == 0 {
		return GlyphUnclassified.Render(profile)
	}
	ns := now.UnixNano()
	if ns < 0 {
		ns = -ns
	}
	i := int(ns/int64(SpinnerPeriod)) % len(frames)
	return frames[i]
}
