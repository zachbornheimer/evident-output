// Package width computes terminal cell widths for display text.
package width

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	escapeByte        = 0x1b
	sgrReset          = "\x1b[0m"
	oscHyperlinkClose = "\x1b]8;;\x1b\\"
)

// VisibleCells returns cell width after stripping ANSI CSI/OSC sequences so
// styled and unstyled visible widths match (TXT-013) and OSC 8 links count as
// zero cells (TXT-014).
func VisibleCells(s string) int {
	return Cells(StripANSI(s))
}

// StripANSI removes CSI, OSC, and other common terminal control sequences.
func StripANSI(s string) string {
	if s == "" || !stringsContainsByte(s, 0x1b) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != escapeByte {
			b.WriteByte(s[i])
			i++
			continue
		}
		i = escapeSequenceEnd(s, i)
	}
	return b.String()
}

func escapeSequenceEnd(s string, start int) int {
	if start+1 >= len(s) {
		return len(s)
	}
	switch s[start+1] {
	case '[': // CSI: ESC [ ... final byte @-~
		for i := start + 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return i + 1
			}
		}
	case ']': // OSC: ESC ] ... BEL or ST (ESC \)
		for i := start + 2; i < len(s); i++ {
			if s[i] == 0x07 {
				return i + 1
			}
			if s[i] == escapeByte && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
		}
	case '(':
		return min(start+3, len(s))
	default:
		return min(start+2, len(s))
	}
	return len(s)
}

func stringsContainsByte(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}

// Cells returns terminal cell width for s using a conservative East-Asian /
// emoji heuristic suitable for CLI layout (TXT-001…006).
func Cells(s string) int {
	if s == "" {
		return 0
	}
	n := 0
	var sequence cellCounter
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		n += sequence.Add(r)
	}
	return n
}

type cellCounter struct {
	previousWidth     int
	emojiPresentation bool
}

func (c *cellCounter) Add(r rune) int {
	switch r {
	case 0xfe0f, 0x20e3:
		if c.previousWidth == 1 && !c.emojiPresentation {
			c.emojiPresentation = true
			return 1
		}
		return 0
	}
	add := RuneCells(r)
	if add > 0 {
		c.previousWidth = add
		c.emojiPresentation = false
	}
	return add
}

// RuneCells returns the display width of a single rune.
func RuneCells(r rune) int {
	if r == 0 {
		return 0
	}
	// Combining marks / variation selectors / ZWJ / ZWNJ: zero width
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
		return 0
	}
	switch r {
	case 0x200d, 0x200c, 0xfe0f, 0xfe0e: // ZWJ, ZWNJ, VS16, VS15
		return 0
	case 0x20e3: // combining enclosing keycap
		return 0
	}
	// Default ignorable / format
	if unicode.Is(unicode.Cf, r) && r != 0x00ad {
		// soft hyphen kept as 1; most Cf are 0
		if r >= 0x200b && r <= 0x200f || r >= 0x202a && r <= 0x202e || r >= 0x2060 && r <= 0x206f {
			return 0
		}
	}
	// Wide: fullwidth forms, CJK, Hangul, emoji blocks (conservative = 2)
	if isWide(r) {
		return 2
	}
	// Control: do not contribute (callers should sanitize first)
	if r < 0x20 || r == 0x7f {
		return 0
	}
	return 1
}

func isWide(r rune) bool {
	// Fullwidth / halfwidth forms
	if r >= 0xff01 && r <= 0xff60 {
		return true
	}
	if r >= 0xffe0 && r <= 0xffe6 {
		return true
	}
	// CJK unified + extensions (coarse ranges used by many terminals)
	if r >= 0x1100 && r <= 0x115f { // Hangul Jamo
		return true
	}
	if r >= 0x2e80 && r <= 0xa4cf {
		return true
	}
	if r >= 0xac00 && r <= 0xd7a3 { // Hangul syllables
		return true
	}
	if r >= 0xf900 && r <= 0xfaff {
		return true
	}
	if r >= 0xfe10 && r <= 0xfe19 {
		return true
	}
	if r >= 0xfe30 && r <= 0xfe6f {
		return true
	}
	if r >= 0x20000 && r <= 0x3fffd {
		return true
	}
	// Emoji (conservative: treat most pictographs as double-width)
	if r >= 0x1f300 && r <= 0x1faff {
		return true
	}
	if r >= 0x2600 && r <= 0x27bf { // misc symbols / dingbats
		// Many are single-width in some fonts; be conservative for layout safety
		if r >= 0x26a0 || r >= 0x2700 {
			return true
		}
	}
	return false
}

// Truncate trims s to at most maxCells, never splitting a multi-cell rune,
// appending "…" when truncated. Combining sequences stay attached to base.
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
	var sequence cellCounter
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		add := sequence.Add(r)
		// Keep combining marks with the base even near the edge.
		if add == 0 && n > 0 {
			b = append(b, s[:size]...)
			s = s[size:]
			continue
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

// TruncateVisible trims styled terminal text to maxCells while retaining
// complete control sequences and closing any active presentation state.
func TruncateVisible(s string, maxCells int) string {
	if maxCells <= 0 {
		return ""
	}
	if VisibleCells(s) <= maxCells {
		return s
	}
	if maxCells == 1 {
		return "…"
	}

	var b strings.Builder
	b.Grow(len(s))
	cells := 0
	hasSGR := false
	hasHyperlink := false
	var visibleSequence cellCounter
	for i := 0; i < len(s); {
		if s[i] == escapeByte {
			end := escapeSequenceEnd(s, i)
			sequence := s[i:end]
			b.WriteString(sequence)
			hasSGR = hasSGR || isSGR(sequence)
			hasHyperlink = hasHyperlink || strings.HasPrefix(sequence, "\x1b]8;")
			i = end
			continue
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		add := visibleSequence.Add(r)
		if add == 0 && cells > 0 {
			b.WriteString(s[i : i+size])
			i += size
			continue
		}
		if cells+add > maxCells-1 {
			break
		}
		b.WriteString(s[i : i+size])
		cells += add
		i += size
	}
	b.WriteString("…")
	if hasHyperlink {
		b.WriteString(oscHyperlinkClose)
	}
	if hasSGR {
		b.WriteString(sgrReset)
	}
	return b.String()
}

func isSGR(sequence string) bool {
	return strings.HasPrefix(sequence, "\x1b[") && strings.HasSuffix(sequence, "m")
}
