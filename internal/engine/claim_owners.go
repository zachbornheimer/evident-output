package engine

import "sync"

// claimOwners records which goroutines hold a resource claim right now, so
// a Wait can see a claim held by the goroutine that started it: the
// errgroup shape, where a claim holder starts a goroutine that waits and
// then blocks on that goroutine. The waiter's own stack holds nothing, so
// only the starting goroutine's identity can say the claim is there.
type claimOwners struct {
	mu   sync.Mutex
	held map[goroutineID]int
}

// hold records one more claim held by g and returns the matching release.
func (c *claimOwners) hold(g goroutineID) (release func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.held == nil {
		c.held = make(map[goroutineID]int)
	}
	c.held[g]++
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.held[g]--; c.held[g] <= 0 {
			delete(c.held, g)
		}
	}
}

// holds reports whether g holds any claim now.
func (c *claimOwners) holds(g goroutineID) bool {
	if g == 0 {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.held[g] > 0
}

// processClaimOwners covers every Output in the process, like
// processResources.
var processClaimOwners claimOwners
