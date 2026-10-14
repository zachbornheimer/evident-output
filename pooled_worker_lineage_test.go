package evo

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

// A goroutine's lineage ends at any goroutine that served as a pooled worker:
// a worker's creator never authored the work the worker ran. k (not after
// make) Defines a1 (after make) while every other worker is busy, so k's kick
// starts a new worker for a1. a1 starts goroutine Y, whose grandchild read
// teaches the lineage Y, the new worker, k's worker. a1 returns and its worker
// exits while k still runs. Y then reads make: it is a1's hands, and a1 is
// ordered after make, so the read is lawful and k, which never read anything,
// must not be failed for it.
func TestLineageChainEndsAtPooledWorkers(t *testing.T) {
	out := initRun(Config{Output: Plain, MaxConcurrency: 8, Facades: Facades{Stderr: io.Discard}})
	defer func() { _ = out.close() }()
	mk := out.Task("make")
	made := occurrence()
	producer := Compute(mk, func(context.Context) (int, error) { defer close(made); return 7, nil })
	release := occurrence()
	hold := func(context.Context) error { <-release; return nil }
	out.Task("hold1").Define(hold)
	out.Task("hold2").Define(hold)
	out.Task("hold3").After(mk).Define(hold)
	a1 := out.Task("a1").After(mk)
	learned, readDone := occurrence(), occurrence()
	var late atomic.Int64

	k := out.Task("k")
	k.Define(func(context.Context) error {
		<-made
		// make has settled and its worker runs hold3, so a1 needs a new worker.
		time.Sleep(attributionSettle / 2)
		a1.Define(func(context.Context) error {
			go func() {
				defer close(readDone)
				grandchild := occurrence()
				go func() { defer close(grandchild); _ = producer.Get() }()
				<-grandchild
				close(learned)
				// By now a1 has returned and the worker that ran it has ended.
				time.Sleep(attributionSettle)
				late.Store(int64(producer.Get()))
			}()
			<-learned
			return nil
		})
		<-readDone
		close(release)
		return nil
	})

	_ = out.finish()

	if late.Load() != 7 || k.wait() != nil || out.err() != nil {
		t.Errorf("late read = %d, k = %v, Err() = %v; want 7, nil, nil", late.Load(), k.wait(), out.err())
	}
}
