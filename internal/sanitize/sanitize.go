// Package sanitize neutralizes untrusted text for terminal-safe display.
package sanitize

import (
	"strings"
	"unicode/utf8"
)

// Text neutralizes control characters and invalid UTF-8 for single-line fields.
// Newlines become spaces. ESC/CSI/OSC and C0 controls other than TAB are
// stripped or replaced. Prefer Block for multi-line evidence (diffs, capture tails).
func Text(s string) string {
	return neutralize(s, false)
}

// Block neutralizes control characters while preserving newlines for multi-line
// presentation fields (Problem.Detail, capture tails). CRLF/CR normalize to LF.
// ESC/CSI and other C0 controls (except TAB and LF) are still stripped.
func Block(s string) string {
	return neutralize(s, true)
}

func neutralize(s string, preserveNewlines bool) string {
	if s == "" {
		return s
	}
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "\uFFFD")
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		switch {
		case r == '\r':
			// Normalize CRLF / bare CR.
			if preserveNewlines {
				if i < len(s) && s[i] == '\n' {
					continue // skip CR; LF handled next
				}
				b.WriteByte('\n')
			} else {
				b.WriteByte(' ')
			}
		case r == '\n':
			if preserveNewlines {
				b.WriteByte('\n')
			} else {
				b.WriteByte(' ')
			}
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
