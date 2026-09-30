package rules

import (
	"regexp"
	"slices"
)

// removedInPhrase is how guidance legitimately names a retired symbol:
// inside a note that says which release removed it.
var removedInPhrase = regexp.MustCompile(`(?i)removed in (\d+\.\d+)`)

// UnexplainedRetired returns every retired Symbol text teaches without also
// saying "removed in <release>" for that Symbol's release. Guidance (a
// remediation, a suggestion) may name a retired symbol only to say it is
// gone; anything else steers the reader to an API that no longer compiles.
func UnexplainedRetired(text string) []RetiredHit {
	var releases []RetiredRelease
	for _, m := range removedInPhrase.FindAllStringSubmatch(text, -1) {
		releases = append(releases, RetiredRelease(m[1]))
	}
	var hits []RetiredHit
	for _, h := range TaughtRetired(text) {
		if !slices.Contains(releases, h.Symbol.RemovedIn) {
			hits = append(hits, h)
		}
	}
	return hits
}
