// This file names whether a Run applies its work or only plans it, because a
// planned Run must never change what freshness remembers.

package freshness

// RunMode is whether a Run applies its work or only plans it (spec §8.2).
type RunMode uint8

const (
	// RunApplies is an ordinary Run: finished Tasks update the manifest.
	RunApplies RunMode = iota
	// RunPlans is a dry run: nothing was applied, so nothing is remembered.
	RunPlans
)

// Remembers reports whether a Run in this mode may change the manifest or
// the per-Task state that feeds it.
func (m RunMode) Remembers() bool { return m == RunApplies }
