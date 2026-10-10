// This file owns the output barrier: the wait that keeps a Basis input from
// being fingerprinted before the operation producing it has settled.

package freshness

import "context"

// BarrierTable is the table of output gates the scheduler keeps, as
// freshness uses it. The caller supplies it (the graph satisfies it), so
// freshness reads gates without importing the scheduler.
type BarrierTable interface {
	// OpenBarrier opens, idempotently, the gate for canonical output path.
	OpenBarrier(path string)
	// SettleBarrier closes path's gate exactly once; a path nobody claimed has
	// no gate and is a no-op.
	SettleBarrier(path string)
	// Barrier is the channel that closes when path's producing operation, if
	// any, has settled. ok is false for a path nobody claimed.
	Barrier(path string) (settled <-chan struct{}, ok bool)
}

// OutputBarrier is a Run's freshness gates (spec §11.6): a barrier, not a
// scheduler edge. The consumer Task itself is never reordered, only one
// Basis observation is delayed until the data it reads is final.
type OutputBarrier struct {
	table BarrierTable
}

// NewOutputBarrier returns the freshness gates held in table.
func NewOutputBarrier(table BarrierTable) OutputBarrier {
	return OutputBarrier{table: table}
}

// Open opens (idempotently) the gate for canonical output path — called the
// moment a Task claims path as a tracked output (spec §11.4/§11.6), before
// that Task's operation has actually run. A later Task consulting the same
// path as a Basis input waits on this gate rather than racing the
// producer's write.
func (b OutputBarrier) Open(path string) { b.table.OpenBarrier(path) }

// Settle closes path's gate exactly once, releasing any waiter (spec
// §11.6): called once the claiming operation has observed its final on-disk
// state for path, whether that operation succeeded, was skipped as current,
// or failed. A path nobody claimed has no gate and is a no-op — a Basis
// input that names a path no Task in this Run produces is never blocked.
func (b OutputBarrier) Settle(path string) { b.table.SettleBarrier(path) }

// Await blocks until path's producing operation (if any) in this Run has
// settled, or ctx is done first.
func (b OutputBarrier) Await(ctx context.Context, path string) error {
	settled, ok := b.table.Barrier(path)
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
