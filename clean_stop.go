package evo

import (
	"context"
	"os"
	"slices"
	"sync"
	"sync/atomic"
)

// cleanStopTask is one running Task that declared CleanStop: the signals
// that end it Succeeded, and the cancel of its own context.
type cleanStopTask struct {
	signals []os.Signal
	cancel  context.CancelFunc
	stopped atomic.Bool
}

func (c *cleanStopTask) names(sig os.Signal) bool {
	return slices.Contains(c.signals, sig)
}

// cleanStopRegistry holds the CleanStop Tasks that are running now. Its zero
// value is ready to use.
type cleanStopRegistry struct {
	mu      sync.Mutex
	running map[*cleanStopTask]struct{}
}

func (r *cleanStopRegistry) add(c *cleanStopTask) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running == nil {
		r.running = map[*cleanStopTask]struct{}{}
	}
	r.running[c] = struct{}{}
}

func (r *cleanStopRegistry) remove(c *cleanStopTask) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.running, c)
}

// stop cancels every running Task that names sig and reports whether any
// did; when none does, the signal belongs to the run.
func (r *cleanStopRegistry) stop(sig os.Signal) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	handled := false
	for c := range r.running {
		if c.names(sig) {
			c.stopped.Store(true)
			c.cancel()
			handled = true
		}
	}
	return handled
}

// wrap runs fn under a context a named signal can cancel, and reports a
// callback stopped that way as success.
func (r *cleanStopRegistry) wrap(signals []os.Signal, fn func(context.Context) error) func(context.Context) error {
	if len(signals) == 0 {
		return fn
	}
	return func(ctx context.Context) error {
		taskCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		task := &cleanStopTask{signals: signals, cancel: cancel}
		r.add(task)
		defer r.remove(task)
		err := fn(taskCtx)
		if task.stopped.Load() {
			return nil
		}
		return err
	}
}
