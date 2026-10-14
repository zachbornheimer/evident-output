// This file owns the one required line a run with misuse prints.

package project

// misuseGlyph prefixes the misuse line the same way the "! already mutated:
// ..." row does: "!" is attention-only in the glyph vocabulary, and a misuse
// line always demands attention.
const misuseGlyph = "!"

// MisuseLine is the line that states a run's first misuse: its corrective
// sentence, never the raw "evo: ..." sentinel text, so the exit code never
// disagrees with what the reader saw (beginner-1).
func MisuseLine(hint string, color bool) string {
	return StyleGlyph(misuseGlyph, SGRYellow, color) + "  " + hint
}
