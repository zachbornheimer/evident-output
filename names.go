package evo

import (
	"fmt"
	"strings"
)

// DefaultVisibleNames is how many names TruncateNames keeps before summarizing.
const DefaultVisibleNames = 3

// TruncateNames joins names for a skip/kept-style summary.
// Empty names yields "". visible <= 0 uses DefaultVisibleNames.
// When more names remain than visible, appends the overflow glyph for
// profile (evo-rec.md's tightened vocabulary: "… +N more", ASCII "... +N
// more") instead of a bare ", +N" that carries no glyph at all.
func TruncateNames(names []string, visible int, profile GlyphProfile) string {
	if len(names) == 0 {
		return ""
	}
	if visible <= 0 {
		visible = DefaultVisibleNames
	}
	if len(names) <= visible {
		return strings.Join(names, ", ")
	}
	shown := names[:visible]
	omitted := len(names) - visible
	return strings.Join(shown, ", ") + fmt.Sprintf(" %s +%d more", glyphOverflow.render(profile), omitted)
}
