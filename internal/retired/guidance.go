package retired

import (
	"regexp"
	"slices"
)

// removedInPhrase is how guidance legitimately names a retired symbol:
// inside a note that says which release removed it. There is no
// release-less alternate: an excuse that names no release would have to
// cover every Symbol's RemovedIn, which erases the release-specific check
// this function exists to make. Text that wants to call a symbol merely
// superseded (not yet removed for an older pin) must still say which
// release removes it, so the excuse stays attributable to that Symbol.
var removedInPhrase = regexp.MustCompile(`(?i)removed in (\d+\.\d+)`)

// UnexplainedIn returns every retired Symbol text teaches without also
// saying "removed in <release>" for that Symbol's release. Guidance (a
// remediation, a suggestion) may name a retired symbol only to say it is
// gone; anything else steers the reader to an API that no longer compiles.
func UnexplainedIn(text string) []Hit {
	var releases []Release
	for _, m := range removedInPhrase.FindAllStringSubmatch(text, -1) {
		releases = append(releases, Release(m[1]))
	}
	var hits []Hit
	for _, h := range TaughtIn(text) {
		if slices.Contains(releases, h.Symbol.RemovedIn) {
			continue
		}
		hits = append(hits, h)
	}
	return hits
}
