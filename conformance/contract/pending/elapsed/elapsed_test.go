// Pending: ZYS-1046 one elapsed format. Red today because the package does
// not compile. GUESSED SIGNATURE: internal/text.FormatElapsed(time.Duration)
// string, the single formatter the live renderer, the plain renderer, and
// the quiet suffix all call (today three shapes exist:
// internal/render/live formatElapsed and formatQuiet, and Duration.String).
package elapsed_test

import (
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/text"
)

func TestElapsedFormatIsOneShapeAcrossRanges(t *testing.T) {
	cases := map[time.Duration]string{
		2 * time.Second:                               "2s",
		4*time.Minute + 12*time.Second:                "4m12s",
		time.Minute + time.Second:                     "1m01s",
		4*time.Minute + 2*time.Second:                 "4m02s",
		time.Minute:                                   "1m",
		time.Hour:                                     "1h",
		3*time.Hour + 4*time.Minute:                   "3h04m",
		24 * time.Hour:                                "1d",
		2*24*time.Hour + 3*time.Hour + 30*time.Minute: "2d3h",
	}
	for d, want := range cases {
		if got := text.FormatElapsed(d); got != want {
			t.Errorf("FormatElapsed(%v) = %q, want %q", d, got, want)
		}
	}
}
