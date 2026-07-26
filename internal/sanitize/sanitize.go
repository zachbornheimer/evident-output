// Package sanitize neutralizes untrusted text for terminal-safe display.
package sanitize

import (
	"strings"
	"unicode/utf8"
)

// Text neutralizes control characters and invalid UTF-8 for display fields.
// Newlines in ordinary fields become spaces. ESC/CSI/OSC and C0 controls
// other than TAB are stripped or replaced.
func Text(s string) string {
	if s == "" {
		return s
	}
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "\uFFFD")
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r':
			b.WriteByte(' ')
		case r == '\t':
			b.WriteByte('\t')
		case r == 0x1b: // ESC
			b.WriteString("^[")
		case r == 0x07, r == 0x08: // BEL, BS
			// drop
		case r < 0x20 || r == 0x7f:
			// other C0
		case r >= 0x80 && r <= 0x9f:
			// C1
		case r == 0x202a || r == 0x202b || r == 0x202c || r == 0x202d || r == 0x202e ||
			r == 0x2066 || r == 0x2067 || r == 0x2068 || r == 0x2069:
			// bidi controls — drop
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
