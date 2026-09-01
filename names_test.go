package evo_test

import (
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestTruncateNames(t *testing.T) {
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
		{name: "four plus", names: []string{"a", "b", "c", "d"}, visible: 3, want: "a, b, c, +1"},
		{name: "five plus", names: []string{"a", "b", "c", "d", "e"}, visible: 3, want: "a, b, c, +2"},
		{name: "visible zero uses default", names: []string{"a", "b", "c", "d"}, visible: 0, want: "a, b, c, +1"},
		{name: "visible negative uses default", names: []string{"a", "b", "c", "d"}, visible: -1, want: "a, b, c, +1"},
		{name: "custom visible", names: []string{"a", "b", "c", "d"}, visible: 2, want: "a, b, +2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := evo.TruncateNames(tt.names, tt.visible)
			if got != tt.want {
				t.Fatalf("TruncateNames(%v, %d) = %q, want %q", tt.names, tt.visible, got, tt.want)
			}
		})
	}
}
