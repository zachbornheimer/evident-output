package engine

import "context"

// openOutputBarrierLocked opens (idempotently) the freshness gate for
// canonical output path — called the moment a Task claims path as a
// tracked output (spec §11.4/§11.6), before that Task's operation has
// actually run. A later Task consulting the same path as a Basis input
// waits on this gate rather than racing the producer's write (see
// graph.Graph.OpenBarrier).
func (o *Output) openOutputBarrierLocked(path string) {
	o.graph.OpenBarrier(path)
}

// settleOutputBarrierLocked closes path's gate exactly once, releasing any
// waiter (spec §11.6): called once the claiming operation has observed its
// final on-disk state for path, whether that operation succeeded, was
// skipped as current, or failed. A path nobody claimed has no gate and is a
// no-op — a Basis input that names a path no Task in this Run produces is
// never blocked.
func (o *Output) settleOutputBarrierLocked(path string) {
	o.graph.SettleBarrier(path)
}

// settleOutputBarrier settles path's gate — the counterpart callers defer
// immediately after a successful claim so the gate always releases exactly
// once when that call returns, on every return path (success, skip, or
// error).
func (o *Output) settleOutputBarrier(path string) {
	o.graph.SettleBarrier(path)
}

// awaitOutputBarrier blocks until path's producing operation (if any) in
// this Run has settled, or ctx is done first (spec §11.6: a barrier, not a
// scheduler edge — the consumer Task itself is never reordered, only this
// one Basis observation is delayed until the data it reads is final).
func (o *Output) awaitOutputBarrier(ctx context.Context, path string) error {
	settled, ok := o.graph.Barrier(path)
	if !ok {
		return nil
	}
	select {
	case <-settled:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
