package evo

import "unicode/utf8"

// truncateUTF8 trims s to at most max bytes without splitting a multi-byte rune,
// then appends suffix when truncation occurs. max is a byte budget for s before suffix.
func truncateUTF8(s string, max int, suffix string) string {
	if max <= 0 {
		if s == "" {
			return ""
		}
		return suffix
	}
	if len(s) <= max {
		return s
	}
	end := max
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	if end == 0 {
		// No complete rune fits — still emit suffix so callers see truncation.
		return suffix
	}
	return s[:end] + suffix
}
