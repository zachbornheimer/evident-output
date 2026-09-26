// Package ordered holds sorted-insertion helpers shared by engine
// subpackages that keep their own key-ordered slices (the ledger's
// sections, the scheduler's tallies) but must not import each other.
package ordered

import (
	"slices"
	"sort"
)

// Insert places item after every element of items (sorted by key) whose
// key is <= key(item), keeping ties in arrival order. items are mostly
// appended in key order already, so the common case is a check against
// the last element; otherwise a binary search finds the slot.
func Insert[T any](items []T, item T, key func(T) int) []T {
	k := key(item)
	if len(items) == 0 || key(items[len(items)-1]) <= k {
		return append(items, item)
	}
	at := sort.Search(len(items), func(i int) bool { return key(items[i]) > k })
	return slices.Insert(items, at, item)
}
