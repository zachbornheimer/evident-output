package engine

import (
	"context"
	"io"
	"testing"
)

// settledTask returns a Task that has already resolved Done.
func settledTask(tb testing.TB) *TaskHandle {
	tb.Helper()
	out := Init(Config{Isolated: true, StateDir: tb.TempDir(), Stdout: io.Discard, Stderr: io.Discard})
	tb.Cleanup(func() { _ = out.Close() })
	task := out.Task("done")
	task.Define(func(context.Context) error { return nil })
	if err := task.Wait(); err != nil {
		tb.Fatalf("Wait: %v", err)
	}
	return task
}

// TestWaitOnSettledTaskDoesNotAllocate proves the fast path is free: with
// no resource claim held anywhere, a Wait on work that already settled
// reads no stack. It used to walk and symbolize the stack twice (about
// 5us and 750 B per walk).
func TestWaitOnSettledTaskDoesNotAllocate(t *testing.T) {
	task := settledTask(t)
	if allocs := testing.AllocsPerRun(100, func() { _ = task.Wait() }); allocs != 0 {
		t.Fatalf("Wait on a settled Task allocated %.0f times per call, want 0", allocs)
	}
}

func BenchmarkWaitTerminalTask(b *testing.B) {
	task := settledTask(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = task.Wait()
	}
}
