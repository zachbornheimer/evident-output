package engine

import (
	"context"
	"sync"
)

// holdKind is the access a public operation takes on one canonical path.
type holdKind int

const (
	holdRead holdKind = iota
	holdWrite
)

// resourceHold is one path occupancy request. path is the canonical
// identity; display is what Doing names while waiting.
type resourceHold struct {
	path    string
	display string
	kind    holdKind
}

// grantedHold is one granted occupancy, keyed by the acquire token so a
// release drops exactly the holds that acquire took.
type grantedHold struct {
	token uint64
	kind  holdKind
}

// resourceGraph is the process-local path coordinator: read+read may
// overlap; read+write and write+write wait. It does not coordinate across
// processes.
type resourceGraph struct {
	mu     sync.Mutex
	cond   *sync.Cond
	held   map[string][]grantedHold
	nextID uint64
}

func newResourceGraph() *resourceGraph {
	g := &resourceGraph{held: make(map[string][]grantedHold)}
	g.cond = sync.NewCond(&g.mu)
	return g
}

// processResources coordinates File/Patch/Exec path occupancy in this
// process. Tests share it; unique t.TempDir paths keep them isolated.
var processResources = newResourceGraph()

func (g *resourceGraph) acquire(ctx context.Context, task *TaskHandle, holds []resourceHold) (func(), error) {
	holds = coalesceHolds(holds)
	if len(holds) == 0 {
		return func() {}, nil
	}
	stop := context.AfterFunc(ctx, func() {
		g.mu.Lock()
		g.cond.Broadcast()
		g.mu.Unlock()
	})
	defer stop()

	announced := false
	g.mu.Lock()
	defer g.mu.Unlock()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if g.canGrantLocked(holds) {
			token := g.grantLocked(holds)
			return func() { g.releaseToken(token) }, nil
		}
		if !announced {
			announced = true
			display := g.blockedDisplayLocked(holds)
			if task != nil && display != "" {
				g.mu.Unlock()
				task.Doing("waiting for %s", display)
				g.mu.Lock()
				continue
			}
		}
		g.cond.Wait()
	}
}

func (g *resourceGraph) canGrantLocked(holds []resourceHold) bool {
	for _, h := range holds {
		if !compatibleHolds(g.held[h.path], h.kind) {
			return false
		}
	}
	return true
}

func (g *resourceGraph) grantLocked(holds []resourceHold) uint64 {
	g.nextID++
	token := g.nextID
	for _, h := range holds {
		g.held[h.path] = append(g.held[h.path], grantedHold{token: token, kind: h.kind})
	}
	return token
}

func (g *resourceGraph) releaseToken(token uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for path, cur := range g.held {
		kept := cur[:0]
		for _, h := range cur {
			if h.token != token {
				kept = append(kept, h)
			}
		}
		if len(kept) == 0 {
			delete(g.held, path)
			continue
		}
		g.held[path] = kept
	}
	g.cond.Broadcast()
}

func (g *resourceGraph) blockedDisplayLocked(holds []resourceHold) string {
	for _, h := range holds {
		if !compatibleHolds(g.held[h.path], h.kind) {
			if h.display != "" {
				return h.display
			}
			return h.path
		}
	}
	if len(holds) == 0 {
		return ""
	}
	if holds[0].display != "" {
		return holds[0].display
	}
	return holds[0].path
}

func compatibleHolds(existing []grantedHold, want holdKind) bool {
	for _, h := range existing {
		if h.kind == holdWrite || want == holdWrite {
			return false
		}
	}
	return true
}

func coalesceHolds(holds []resourceHold) []resourceHold {
	if len(holds) == 0 {
		return nil
	}
	order := make([]string, 0, len(holds))
	best := make(map[string]resourceHold, len(holds))
	for _, h := range holds {
		if h.path == "" {
			continue
		}
		prev, seen := best[h.path]
		if !seen {
			order = append(order, h.path)
			best[h.path] = h
			continue
		}
		if h.kind == holdWrite {
			prev.kind = holdWrite
			best[h.path] = prev
		}
	}
	out := make([]resourceHold, 0, len(order))
	for _, path := range order {
		out = append(out, best[path])
	}
	return out
}
