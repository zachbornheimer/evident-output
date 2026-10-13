package text

import (
	"time"

	"github.com/zachbornheimer/evident-output/internal/project"
)

// Presentation primitives live in internal/project. These names stay so
// engine call sites need no edit yet; slice 8 deletes this file.

type GlyphProfile = project.GlyphProfile

const (
	GlyphsAuto          = project.GlyphsAuto
	GlyphsUnicode       = project.GlyphsUnicode
	GlyphsASCII         = project.GlyphsASCII
	SpinnerPeriod       = project.SpinnerPeriod
	SGRReset            = project.SGRReset
	SGRBold             = project.SGRBold
	SGRDim              = project.SGRDim
	SGRRed              = project.SGRRed
	SGRGreen            = project.SGRGreen
	SGRYellow           = project.SGRYellow
	SGRCyan             = project.SGRCyan
	SGRBlue             = project.SGRBlue
	DefaultVisibleNames = project.DefaultVisibleNames
)

var (
	GlyphDone         = project.GlyphDone
	GlyphFailedState  = project.GlyphFailedState
	GlyphBlockedState = project.GlyphBlockedState
	GlyphWarningState = project.GlyphWarningState
	GlyphCancelled    = project.GlyphCancelled
	GlyphNotStarted   = project.GlyphNotStarted
	GlyphSkipDetail   = project.GlyphSkipDetail
	GlyphPending      = project.GlyphPending
	GlyphRunning      = project.GlyphRunning
	GlyphHumanInput   = project.GlyphHumanInput
	GlyphNextAction   = project.GlyphNextAction
	GlyphEvidence     = project.GlyphEvidence
	GlyphOverflow     = project.GlyphOverflow
	GlyphUnclassified = project.GlyphUnclassified
	GlyphHeartbeat    = project.GlyphHeartbeat
)

func VisibleCells(s string) int { return project.VisibleCells(s) }

func StripANSI(s string) string { return project.StripANSI(s) }

func Cells(s string) int { return project.Cells(s) }

func RuneCells(r rune) int { return project.RuneCells(r) }

func Truncate(s string, maxCells int) string { return project.Truncate(s, maxCells) }

func TruncateVisible(s string, maxCells int) string { return project.TruncateVisible(s, maxCells) }

func Style(s, code string, color bool) string { return project.ApplySGR(s, code, color) }

func StyleGlyph(glyph, code string, color bool) string {
	return project.StyleGlyph(glyph, code, color)
}

func Dim(s string, color bool) string { return project.Dim(s, color) }

func PadRight(s string, n int) string { return project.PadRight(s, n) }

func PadLeft(s string, n int) string { return project.PadLeft(s, n) }

func FormatElapsed(d time.Duration) string { return project.FormatElapsed(d) }

func Pluralize(quantity int64, singular string) string {
	return project.Pluralize(quantity, singular)
}

func IsPlural(noun string) bool { return project.IsPlural(noun) }

func ConjugatePast(verb string) string { return project.ConjugatePast(verb) }

func TruncateNames(names []string, visible int, profile ...GlyphProfile) string {
	return project.TruncateNames(names, visible, profile...)
}

func SpinnerFrames(profile GlyphProfile) []string { return project.SpinnerFrames(profile) }

func SpinnerGlyph(now time.Time, profile GlyphProfile) string {
	return project.SpinnerGlyph(now, profile)
}
