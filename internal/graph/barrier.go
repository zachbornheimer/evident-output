package graph

// OpenBarrier opens (idempotently) the freshness gate for canonical output
// path — called the moment a Task claims path as a tracked output, before
// that Task's operation has actually run. A later Task consulting the same
// path as a Basis input waits on this gate rather than racing the
// producer's write.
func (g *Graph) OpenBarrier(path string) {
	g.lock()
	defer g.unlock()
	if g.barriers == nil {
		g.barriers = make(map[string]chan struct{})
	}
	if _, exists := g.barriers[path]; !exists {
		g.barriers[path] = make(chan struct{})
	}
}

// SettleBarrier closes path's gate exactly once, releasing any waiter:
// called once the claiming operation has observed its final on-disk state
// for path, whether that operation succeeded, was skipped as current, or
// failed. A path nobody claimed has no gate and is a no-op — a Basis input
// that names a path no Task in this Run produces is never blocked.
func (g *Graph) SettleBarrier(path string) {
	g.lock()
	defer g.unlock()
	ch, ok := g.barriers[path]
	if !ok {
		return
	}
	select {
	case <-ch:
		// Already settled (defensive: settle must only be called once per
		// claim, but a double-call must never panic on a closed channel).
	default:
		close(ch)
	}
}

// Barrier is the channel that closes when path's producing operation, if
// any, has settled. ok is false for a path nobody claimed.
func (g *Graph) Barrier(path string) (settled <-chan struct{}, ok bool) {
	g.lock()
	defer g.unlock()
	ch, ok := g.barriers[path]
	return ch, ok
}
