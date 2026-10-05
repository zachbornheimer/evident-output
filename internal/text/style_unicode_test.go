package text

import "testing"

// TestPadRight_UsesDisplayCellWidth pins §18's Unicode-subject requirement:
// padding is computed from terminal display width, not byte length, so a
// wide-glyph (CJK) subject pads to the same visual column as an ASCII one.
func TestPadRight_UsesDisplayCellWidth(t *testing.T) {
	// "分支" is 2 runes / 6 bytes but 4 display cells (2 wide glyphs).
	// Padding it to width 6 should add 2 spaces (6-4), not 0 (6-6 bytes).
	got := PadRight("分支", 6)
	want := "分支  "
	if got != want {
		t.Fatalf("PadRight(%q, 6) = %q, want %q", "分支", got, want)
	}
}

func TestPadLeft_UsesDisplayCellWidth(t *testing.T) {
	got := PadLeft("分支", 6)
	want := "  分支"
	if got != want {
		t.Fatalf("PadLeft(%q, 6) = %q, want %q", "分支", got, want)
	}
}
