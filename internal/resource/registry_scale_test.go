package resource

import (
	"context"
	"runtime"
	"sync"
	"testing"
)

// grantVisitsPerClaim bounds granting work per claim: a constant number of
// comparisons, never a rescan of every waiter per release.
const grantVisitsPerClaim = 8

// TestContendedDrainIsLinear pins the zq shape (many worktree Effects on
// one repository root): n writers claiming one key drain in linear work.
func TestContendedDrainIsLinear(t *testing.T) {
	const n = 2000
	r := NewRegistry()
	claim := Claim{Key: Key{space: spaceLogical, name: "repo"}, Mode: Write}
	gate, holding := make(chan struct{}), make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		_ = r.Hold(context.Background(), claim, func(context.Context) error {
			close(holding)
			<-gate
			return nil
		})
	})
	<-holding
	for range n {
		wg.Go(func() {
			_ = r.Hold(context.Background(), claim, func(context.Context) error { return nil })
		})
	}
	for _, waiting := r.occupancy(); waiting < n; _, waiting = r.occupancy() {
		runtime.Gosched()
	}
	close(gate)
	wg.Wait()
	r.mu.Lock()
	visits := r.visits
	r.mu.Unlock()
	t.Logf("n=%d visits=%d", n, visits)
	if visits > grantVisitsPerClaim*n {
		t.Fatalf("granting %d contended claims made %d comparisons (want <= %d)", n, visits, grantVisitsPerClaim*n)
	}
}
