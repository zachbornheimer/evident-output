package text_test

import (
	"regexp"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/text"
)

func TestFormatElapsed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{-5 * time.Second, "0s"},
		{999 * time.Millisecond, "0s"},
		{1500 * time.Millisecond, "1s"},
		{59 * time.Second, "59s"},
		{60 * time.Second, "1m0s"},
		{4*time.Minute + 12*time.Second, "4m12s"},
		{59*time.Minute + 59*time.Second, "59m59s"},
		{60 * time.Minute, "1h00m"},
		{3*time.Hour + 4*time.Minute, "3h04m"},
		{23*time.Hour + 59*time.Minute, "23h59m"},
		{24 * time.Hour, "1d0h"},
		{25*time.Hour + 30*time.Minute, "1d1h"},
		{48 * time.Hour, "2d0h"},
		{2*24*time.Hour + 3*time.Hour + 30*time.Minute, "2d3h"},
	}
	for _, tc := range cases {
		if got := text.FormatElapsed(tc.d); got != tc.want {
			t.Errorf("FormatElapsed(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

var (
	compactElapsed = regexp.MustCompile(`^(\d+s|\d+m\d+s|\d+h\d{2}m|\d+d\d+h)$`)
	goStringTail   = regexp.MustCompile(`h\d+m\d+s`)
)

func TestFormatElapsedAlwaysCompactWithUnits(t *testing.T) {
	t.Parallel()
	for d := time.Duration(0); d < 72*time.Hour; d += 7*time.Minute + 13*time.Second {
		got := text.FormatElapsed(d)
		if !compactElapsed.MatchString(got) {
			t.Fatalf("FormatElapsed(%v) = %q, not the compact unit-bearing form", d, got)
		}
		if goStringTail.MatchString(got) {
			t.Fatalf("FormatElapsed(%v) = %q, has a Duration.String tail", d, got)
		}
	}
}
