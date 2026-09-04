package text

import "strings"

// SGR styles for terminal projection (library-owned sequences only).
const (
	SGRReset  = "\x1b[0m"
	SGRBold   = "\x1b[1m"
	SGRDim    = "\x1b[2m"
	SGRRed    = "\x1b[31m"
	SGRGreen  = "\x1b[32m"
	SGRYellow = "\x1b[33m"
	SGRCyan   = "\x1b[36m"
	SGRBlue   = "\x1b[34m"
)

// Style wraps s in code (an SGR escape) when color is true and both s and
// code are non-empty, closing with SGRReset.
func Style(s, code string, color bool) string {
	if !color || code == "" || s == "" {
		return s
	}
	return code + s + SGRReset
}

// StyleGlyph applies Style to a rendered glyph face.
func StyleGlyph(glyph, code string, color bool) string {
	return Style(glyph, code, color)
}

// Dim applies the dim SGR style.
func Dim(s string, color bool) string {
	return Style(s, SGRDim, color)
}

// PadRight right-pads s with spaces to width n.
func PadRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

// PadLeft left-pads s with spaces to width n.
func PadLeft(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return strings.Repeat(" ", n-len(s)) + s
}
