package evo_test

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output/v2"
	"github.com/zachbornheimer/evident-output/v2/testkit/scaletest"
)

// group1kSize is the throughput workload's width: enough trivial Tasks that
// per-callback bookkeeping, not run setup, dominates the cost.
const group1kSize = 1000

// group1kNames are the workload's Task names, built once so the measurement
// is evo's cost, not fmt's.
var group1kNames = func() []string {
	names := make([]string, group1kSize)
	for i := range names {
		names[i] = fmt.Sprintf("item %d", i)
	}
	return names
}()

// runGroup1kPlain declares group1kSize trivial Defines in one Group, Plain
// output to io.Discard, and runs them to the run's end.
func runGroup1kPlain(tb testing.TB) {
	tb.Helper()
	cfg := evo.Config{
		Name: "bench", Output: evo.Plain, State: evo.State{Dir: tb.TempDir()},
		Facades: evo.Facades{Stdout: io.Discard, Stderr: io.Discard},
	}
	result := evo.Execute(context.Background(), cfg, func(_ context.Context, r *evo.Run) error {
		items := r.Group("items")
		for _, name := range group1kNames {
			items.Task(name).Define(func(context.Context) error { return nil })
		}
		return nil
	})
	if code := result.ExitCode(); code != 0 {
		tb.Fatalf("run exited %d, want 0: %v", code, result.Err)
	}
}

func BenchmarkGroup1kPlain(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		runGroup1kPlain(b)
	}
}

// group1kPlainBound is the wall time 1,000 trivial Tasks in one Group may
// take on an idle host. v1.0.0 ran this workload in about 25 ms here; the
// bound leaves room for a slower machine while still catching a per-callback
// convoy on the Run's lock, which cost 2.0 several times that.
const group1kPlainBound = 200 * time.Millisecond

// TestGroup1kPlainStaysUnder200ms guards the throughput the benchmark
// measures. Wall time only means something on an idle host, so a loaded one
// skips rather than fails; the fastest of several runs is the estimate.
func TestGroup1kPlainStaysUnder200ms(t *testing.T) {
	if busy, reason := scaletest.HostIsBusy(); busy {
		t.Skip(reason)
	}
	elapsed := scaletest.Fastest(scaletest.Samples, func() time.Duration {
		return scaletest.Elapsed(func() { runGroup1kPlain(t) })
	})
	if elapsed > group1kPlainBound {
		t.Fatalf("1,000 trivial Tasks in one Plain Group took %v, want <= %v", elapsed, group1kPlainBound)
	}
}
