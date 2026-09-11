package engine

import txt "github.com/zachbornheimer/evident-output/internal/text"

// DefaultVisibleNames is how many names TruncateNames keeps before summarizing.
const DefaultVisibleNames = txt.DefaultVisibleNames

// TruncateNames joins names for a skip/kept-style summary.
// Empty names yields "". visible <= 0 uses DefaultVisibleNames.
// When more names remain than visible, appends the overflow glyph for
// profile (evo-rec.md's tightened vocabulary: "… +N more", ASCII "... +N
// more") instead of a bare ", +N" that carries no glyph at all.
// profile is variadic so the simplest call — TruncateNames(names, visible) —
// stays correct: an omitted profile renders the Unicode overflow glyph; a
// caller that has already resolved a GlyphProfile passes it explicitly.
func TruncateNames(names []string, visible int) string {
	return txt.TruncateNames(names, visible)
}
