//go:build !evo_pre1382

package evo_test

// firstRune returns s's first rune as a string ("" for an empty s). Its
// canonical home, scenario_test.go, is quarantined behind evo_pre1382 (see
// docs/zys-1382/KNOWN_BROKEN.md); this copy serves the live tests that still
// build without the tag and disappears when the tag brings the original back.
func firstRune(s string) string {
	for _, r := range s {
		return string(r)
	}
	return ""
}
