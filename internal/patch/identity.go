package patch

import (
	"crypto/sha256"
	"fmt"
)

// identityDomain separates an edit's identity from every other sha256 the
// module computes.
const identityDomain = "evident-output:patch:edit:v1\x00"

// Identity is the sha256 of everything that determines what f does to a
// source: its path, whether it creates, the mode it sets, and every hunk
// line. Two edits share an Identity exactly when they are the same edit.
func (f File) Identity() [sha256.Size]byte {
	h := sha256.New()
	_, _ = h.Write([]byte(identityDomain))
	_, _ = fmt.Fprintf(h, "%q %t %t %q %o\x00", f.Path, f.Create, f.Delete, f.From, f.Mode)
	for _, hunk := range f.Hunks {
		_, _ = fmt.Fprintf(h, "@@ %d,%d %d,%d\x00", hunk.OldStart, hunk.OldCount, hunk.NewStart, hunk.NewCount)
		for _, line := range hunk.lines {
			_, _ = fmt.Fprintf(h, "%c%t%q\x00", line.op, line.eol, line.text)
		}
	}
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	return sum
}
