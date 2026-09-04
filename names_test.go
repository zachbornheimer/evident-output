package evo_test

import (
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestTruncateNames_UnicodeOverflow is red-first against evo-rec.md's
// tightened glyph vocabulary (item 6): overflow renders as "… +N more", not
// a bare ", +N" with no glyph at all.
func TestTruncateNames_UnicodeOverflow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		names   []string
		visible int
		want    string
	}{
		{name: "empty", names: nil, visible: 3, want: ""},
		{name: "empty slice", names: []string{}, visible: 3, want: ""},
		{name: "three exact", names: []string{"a", "b", "c"}, visible: 3, want: "a, b, c"},
		{name: "under visible", names: []string{"a", "b"}, visible: 3, want: "a, b"},
		{name: "four plus", names: []string{"a", "b", "c", "d"}, visible: 3, want: "a, b, c … +1 more"},
		{name: "five plus", names: []string{"a", "b", "c", "d", "e"}, visible: 3, want: "a, b, c … +2 more"},
		{name: "visible zero uses default", names: []string{"a", "b", "c", "d"}, visible: 0, want: "a, b, c … +1 more"},
		{name: "visible negative uses default", names: []string{"a", "b", "c", "d"}, visible: -1, want: "a, b, c … +1 more"},
		{name: "custom visible", names: []string{"a", "b", "c", "d"}, visible: 2, want: "a, b … +2 more"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := evo.TruncateNames(tt.names, tt.visible, evo.GlyphsUnicode)
			if got != tt.want {
				t.Fatalf("TruncateNames(%v, %d) = %q, want %q", tt.names, tt.visible, got, tt.want)
			}
		})
	}
}

// TestTruncateNames_TwoArgCallDefaultsToUnicode is red-first against the N2
// regression that forced a mandatory GlyphProfile arg onto TruncateNames,
// breaking every existing 2-arg caller (e.g. zq's setup_python.go). The
// simplest call — TruncateNames(names, visible) — must still compile and
// must default to the Unicode overflow glyph.
func TestTruncateNames_TwoArgCallDefaultsToUnicode(t *testing.T) {
	t.Parallel()
	got := evo.TruncateNames([]string{"a", "b", "c", "d"}, 0)
	want := "a, b, c … +1 more"
	if got != want {
		t.Fatalf("TruncateNames(2-arg) = %q, want %q", got, want)
	}
}

// TestTruncateNames_ASCIIOverflow proves the ASCII glyph profile downgrades
// the overflow marker to "..." with the same "+N more" wording — identical
// semantics, degraded glyph only (GLYPH-001).
func TestTruncateNames_ASCIIOverflow(t *testing.T) {
	t.Parallel()
	got := evo.TruncateNames([]string{"a", "b", "c", "d"}, 3, evo.GlyphsASCII)
	want := "a, b, c ... +1 more"
	if got != want {
		t.Fatalf("TruncateNames(ASCII) = %q, want %q", got, want)
	}
}
