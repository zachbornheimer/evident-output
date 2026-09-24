package resource

import (
	"context"
	"strconv"
	"sync"
	"testing"
)

// BenchmarkRegistryContended drains n writers that all claim one key, the
// zq shape of many worktree Effects claiming one repository root. A
// release wakes only the claim it grants and rechecks only the waiters it
// could unblock (regrantLocked), so draining is linear in n;
// TestContendedDrainIsLinear pins that by counting comparisons.
func BenchmarkRegistryContended(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			claim := Claim{Key: Key{space: spaceLogical, name: "repo"}, Mode: Write}
			for b.Loop() {
				r := NewRegistry()
				var wg sync.WaitGroup
				for range n {
					wg.Go(func() {
						_ = r.Hold(context.Background(), claim, func(context.Context) error { return nil })
					})
				}
				wg.Wait()
			}
		})
	}
}
