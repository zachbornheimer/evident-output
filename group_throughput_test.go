package evo_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output/v2"
	"github.com/zachbornheimer/evident-output/v2/internal/graph"
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

// v1GroupBytesPerOp is v1.0.0's BenchmarkGroup1kPlain allocation, 7.39 MB/op
// (7,388,988 to 7,408,393 B/op across runs). 2.0 allocates about 7.14 MB, so
// the bound has room for B/op noise (about 0.3% between runs).
const v1GroupBytesPerOp = 7_390_000

// TestGroup1kPlainBytesPerOpUnderV1 holds the 1,000-Task Plain Group to the
// bytes v1.0.0 allocated for it. Bytes per op do not depend on host load, so
// unlike the wall-time guard above this one never skips for load. The race
// detector does inflate them (about 7.9 MB/op), so a -race build skips; the
// plain run enforces the bound.
func TestGroup1kPlainBytesPerOpUnderV1(t *testing.T) {
	if scaletest.RaceDetector {
		t.Skip("the race detector instruments allocations and inflates bytes/op; the plain run enforces the v1.0.0 bound")
	}
	result := testing.Benchmark(BenchmarkGroup1kPlain)
	if got := result.AllocedBytesPerOp(); got > v1GroupBytesPerOp {
		t.Fatalf("1,000 trivial Tasks in one Plain Group allocated %d B/op (%d allocs/op), want <= v1.0.0's %d",
			got, result.AllocsPerOp(), v1GroupBytesPerOp)
	}
}

// v1GroupNsPerOp is v1.0.0's BenchmarkGroup1kPlain wall time: 8.97 ms/op, the
// median of twelve runs (8.76 to 9.54 ms) on an Apple M1 Max, ten cores,
// Go 1.27.2, with the host's one-minute load average between 4.9 and 6.1.
// The round-5 runtime review measured it back to back with 2.0.
const v1GroupNsPerOp = 8_970_000

// group1kWallMargin is how much slower than v1.0.0 the same Group may run:
// within 20 percent.
const group1kWallMargin = 1.2

// group1kBusyLoad is the one-minute load average above which the wall-time
// bound says nothing: v1GroupNsPerOp was recorded at a load of about 5, and a
// busier host slows v1.0.0 as much as 2.0.
const group1kBusyLoad = 10

// group1kRunsPerSample is how many Groups one timed sample runs, enough that
// the timer's resolution and one slow start are a small share of it.
const group1kRunsPerSample = 10

// group1kMaxSamples is how many samples the bound gets to be met in. Host load
// comes in bursts, such as the compile that ran just before the test, and a
// burst only ever adds time, so a regression is a bound that none of them meet.
const group1kMaxSamples = 4 * scaletest.CheapSamples

// perfBaselineEnv names the switch that says the host is the one
// v1GroupNsPerOp was recorded on, or as fast: `mise run perf` sets it.
const perfBaselineEnv = "EVO_PERF_BASELINE"

// TestGroup1kPlainWallTimeWithinTwentyPercentOfV1 holds the 1,000-Task Plain
// Group to the wall time v1.0.0 took for it, v1GroupNsPerOp, plus the
// margin. An absolute wall time means something only on the host that
// recorded it, so the test runs only where perfBaselineEnv is set. The fastest
// sample is the estimate: host load only adds time. A busy host skips,
// because the baseline was recorded on a quiet one; the race detector skips,
// because it slows every Task.
func TestGroup1kPlainWallTimeWithinTwentyPercentOfV1(t *testing.T) {
	if os.Getenv(perfBaselineEnv) != "1" {
		t.Skip("machine-specific baseline; run via mise run perf")
	}
	if !graph.WorkerReuseAvailable() {
		t.Skip("worker reuse needs the /sched/goroutines-created runtime metric (Go 1.27); this toolchain runs every Task on its own goroutine, which the v1.0.0 bound does not describe")
	}
	if scaletest.RaceDetector {
		t.Skip("the race detector slows every Task several times over; the plain run enforces the v1.0.0 bound")
	}
	if busy, reason := scaletest.HostLoadExceeds(group1kBusyLoad); busy {
		t.Skip(reason)
	}
	bound := time.Duration(v1GroupNsPerOp * group1kWallMargin)
	perOp := scaletest.FastestUnder(group1kMaxSamples, bound, func() time.Duration {
		return scaletest.Elapsed(func() {
			for range group1kRunsPerSample {
				runGroup1kPlain(t)
			}
		}) / group1kRunsPerSample
	})
	t.Logf("1,000 trivial Tasks in one Plain Group: %v/op, bound %v", perOp, bound)
	if perOp > bound {
		t.Fatalf("1,000 trivial Tasks in one Plain Group took %v/op, want <= %v (v1.0.0's %v plus 20%%)",
			perOp, bound, time.Duration(v1GroupNsPerOp))
	}
}
