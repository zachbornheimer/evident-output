// Package width computes terminal cell widths for display text.
package width

import "unicode/utf8"

// Cells returns a simple terminal cell width estimate for s.
// ASCII is 1; invalid UTF-8 replacement is 1; other runes default to 1
// (CJK dual-width can be layered later with a full EastAsianWidth table).
func Cells(s string) int {
	if s == "" {
		return 0
	}
	n := 0
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			n++
			s = s[1:]
			continue
		}
		// Combining marks: width 0
		if r >= 0x0300 && r <= 0x036f {
			s = s[size:]
			continue
		}
		n++
		s = s[size:]
	}
	return n
}

// Truncate trims s to at most maxCells, appending "…" when truncated.
func Truncate(s string, maxCells int) string {
	if maxCells <= 0 {
		return ""
	}
	if Cells(s) <= maxCells {
		return s
	}
	if maxCells == 1 {
		return "…"
	}
	var b []byte
	n := 0
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		add := 1
		if r >= 0x0300 && r <= 0x036f {
			add = 0
		}
		if n+add > maxCells-1 {
			break
		}
		b = append(b, s[:size]...)
		n += add
		s = s[size:]
	}
	return string(b) + "…"
}
