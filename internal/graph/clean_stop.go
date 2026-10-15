package graph

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/v2/internal/process"
)

// CleanStops are the running callbacks that end Succeeded when a signal they
// named arrives (evo.CleanStop): a long-running server stopped by SIGTERM
// did its work. The zero value is ready to use.
type CleanStops struct {
	mu      sync.Mutex
	running map[*cleanStop]struct{}
}

// cleanStop is one running callback: the signals that stop it cleanly and
// the cancel of its own context.
type cleanStop struct {
	signals []process.Signal
	cancel  context.CancelFunc
	stopped atomic.Bool
}

// Wrap runs fn under a context one of signals can cancel, and reports a
// callback stopped that way as success. Without signals fn runs as is.
func (r *CleanStops) Wrap(signals []process.Signal, fn func(context.Context) error) func(context.Context) error {
	if len(signals) == 0 {
		return fn
	}
	return func(ctx context.Context) error {
		taskCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		c := &cleanStop{signals: signals, cancel: cancel}
		r.track(c, true)
		defer r.track(c, false)
		err := fn(taskCtx)
		if c.stopped.Load() {
			return nil
		}
		return err
	}
}

// Stop cancels every running callback that named sig and reports whether
// any did; when none did, the signal belongs to the run.
func (r *CleanStops) Stop(sig process.Signal) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	handled := false
	for c := range r.running {
		if slices.Contains(c.signals, sig) {
			c.stopped.Store(true)
			c.cancel()
			handled = true
		}
	}
	return handled
}

// track adds c to the running callbacks, or removes it.
func (r *CleanStops) track(c *cleanStop, running bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !running {
		delete(r.running, c)
		return
	}
	if r.running == nil {
		r.running = make(map[*cleanStop]struct{})
	}
	r.running[c] = struct{}{}
}
