package find

import (
	"context"
	"slices"
	"strings"
	"sync"
)

// Coalescer shares one traversal among identical concurrent searches. The
// traversal outlives any single caller and stops only when every caller
// has left, so one cancelled searcher cannot fail its peers.
type Coalescer struct {
	mu   sync.Mutex
	runs map[string]*run
}

type run struct {
	cancel  context.CancelFunc
	done    chan struct{}
	waiters int
	paths   []string
	err     error
}

// Key identifies a search: scope separates facades, then root and names.
func Key(scope any, root string, names []string) string {
	sorted := slices.Clone(names)
	slices.Sort(sorted)
	sorted = slices.Compact(sorted)
	return strings.Join(append([]string{pointerKey(scope), root}, sorted...), "\x00")
}

// Do runs fn once per key among concurrent callers and hands each caller its
// own copy of the result.
func (c *Coalescer) Do(ctx context.Context, key string, fn func(context.Context) ([]string, error)) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.runs == nil {
		c.runs = map[string]*run{}
	}
	r, ok := c.runs[key]
	if !ok {
		runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		r = &run{cancel: cancel, done: make(chan struct{})}
		c.runs[key] = r
		go func() {
			paths, err := fn(runCtx)
			c.mu.Lock()
			r.paths, r.err = paths, err
			if c.runs[key] == r {
				delete(c.runs, key)
			}
			c.mu.Unlock()
			cancel()
			close(r.done)
		}()
	}
	r.waiters++
	c.mu.Unlock()

	select {
	case <-r.done:
		c.leave(key, r)
		return slices.Clone(r.paths), r.err
	case <-ctx.Done():
		c.leave(key, r)
		return nil, ctx.Err()
	}
}

func (c *Coalescer) leave(key string, r *run) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r.waiters--
	if r.waiters == 0 {
		if c.runs[key] == r {
			delete(c.runs, key)
		}
		r.cancel()
	}
}
