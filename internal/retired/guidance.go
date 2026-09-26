package retired

import (
	"regexp"
	"slices"
)

// removedInPhrase is how guidance legitimately names a retired symbol:
// inside a note that says which release removed it. "superseded" alone
// (no release number) is the one accepted alternate: rec-only guidance
// aimed at an older pin, where the name is not yet retired for that
// dialect, uses it instead of a false "removed in 1.1" — but since it
// names no release, UnexplainedIn cannot attribute it to a specific
// Symbol's RemovedIn, so it is treated as covering every release.
var removedInPhrase = regexp.MustCompile(`(?i)removed in (\d+\.\d+)|(superseded)`)

// UnexplainedIn returns every retired Symbol text teaches without also
// saying "removed in <release>" for that Symbol's release. Guidance (a
// remediation, a suggestion) may name a retired symbol only to say it is
// gone; anything else steers the reader to an API that no longer compiles.
func UnexplainedIn(text string) []Hit {
	var releases []Release
	allReleases := false
	for _, m := range removedInPhrase.FindAllStringSubmatch(text, -1) {
		if m[1] == "" {
			// The "superseded" alternate names no release, so it excuses
			// every Symbol's RemovedIn rather than one in particular.
			allReleases = true
			continue
		}
		releases = append(releases, Release(m[1]))
	}
	var hits []Hit
	for _, h := range TaughtIn(text) {
		if allReleases || slices.Contains(releases, h.Symbol.RemovedIn) {
			continue
		}
		hits = append(hits, h)
	}
	return hits
}
