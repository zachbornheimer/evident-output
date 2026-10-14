package evo

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
)

// failedWithUnorderedRead fails the test unless task settled Failed with a
// summary naming ErrComputedUnordered.
func failedWithUnorderedRead(t *testing.T, task *Task) {
	t.Helper()
	if err := task.wait(); err == nil {
		t.Fatalf("%s settled successfully, want it failed by the unordered read", task.snapshot().Name)
	}
	snap := task.snapshot()
	if snap.State != failed || !strings.Contains(snap.Summary, ErrComputedUnordered.Error()) {
		t.Fatalf("%s = %v %q, want failed naming ErrComputedUnordered", snap.Name, snap.State, snap.Summary)
	}
}

// An unordered Get fails the run with ErrComputedUnordered, and the callback
// that made it does not carry on with a zero value.
func TestCompute_UnorderedGetFailsRunInDefaultMode(t *testing.T) {
	out := newQuietOutput(t, false)
	makeTask := out.Group("producers").Task("make")
	producer := Compute(makeTask, func(context.Context) (int, error) { return 1, nil })
	var continued atomic.Bool
	read := out.Sequence("consumers").Task("read")
	read.Define(func(context.Context) error {
		_ = makeTask.wait()
		_ = producer.Get()
		continued.Store(true)
		return nil
	})
	_ = out.finish()
	if !errors.Is(out.err(), ErrComputedUnordered) {
		t.Fatalf("Err() = %v, want ErrComputedUnordered", out.err())
	}
	if continued.Load() {
		t.Fatal("callback continued past an unordered Get")
	}
	failedWithUnorderedRead(t, read)
}

// TestUnorderedGetIsRefusedInEveryMode pins CONTRACT.md §2: an unordered Get
// fails the run with ErrComputedUnordered in every mode, dry-run included,
// whether or not the producer settled first. The order is declared, not
// observed, so neither a read that raced ahead of the producer nor one that
// waited for it ever yields a value the callback can carry on with.
func TestUnorderedGetIsRefusedInEveryMode(t *testing.T) {
	const attempts = 20
	modes := map[string]runMode{
		"default": {},
		"strict":  {tune: tuning{strict: true}},
		"dry run": {cfg: Config{DryRun: true}},
	}
	reads := map[string]func(*testing.T, runMode){
		"before the producer settles": assertUnorderedGetBeforeSettleUnwinds,
		"after the producer settled":  assertUnorderedGetAfterSettleUnwinds,
	}
	for name, mode := range modes {
		for when, assert := range reads {
			t.Run(name+"/"+when, func(t *testing.T) {
				mode.cfg.Output, mode.cfg.Facades.Stderr = Plain, io.Discard
				for range attempts {
					assert(t, mode)
				}
			})
		}
	}
}

// runMode is one way to run: the Config and tuning a test builds the Run with.
type runMode struct {
	cfg  Config
	tune tuning
}

func assertUnorderedGetBeforeSettleUnwinds(t *testing.T, mode runMode) {
	t.Helper()
	out := initRun(mode.cfg, mode.tune)
	defer func() { _ = out.close() }()
	release := make(chan struct{})
	producer := Compute(out.Group("producers").Task("make"), func(context.Context) (int, error) {
		<-release
		return 1, nil
	})
	var continued atomic.Bool
	read := out.Group("consumers").Task("read")
	read.Define(func(context.Context) error {
		defer close(release)
		_ = producer.Get()
		continued.Store(true)
		return nil
	})
	assertReadUnwound(t, out, read, &continued)
}

// assertUnorderedGetAfterSettleUnwinds reads after the producer settled: the
// callback proved settledness, not order, so Get still fails.
func assertUnorderedGetAfterSettleUnwinds(t *testing.T, mode runMode) {
	t.Helper()
	out := initRun(mode.cfg, mode.tune)
	defer func() { _ = out.close() }()
	made := out.Group("producers").Task("make")
	producer := Compute(made, func(context.Context) (int, error) { return 1, nil })
	var continued atomic.Bool
	read := out.Group("consumers").Task("read")
	read.Define(func(context.Context) error {
		_ = made.wait()
		_ = producer.Get()
		continued.Store(true)
		return nil
	})
	assertReadUnwound(t, out, read, &continued)
}

// assertReadUnwound finishes out and fails the test unless read's callback
// unwound at its Get and the run failed with ErrComputedUnordered.
func assertReadUnwound(t *testing.T, out *Run, read *Task, continued *atomic.Bool) {
	t.Helper()
	_ = out.finish()
	if continued.Load() || !errors.Is(out.err(), ErrComputedUnordered) {
		t.Fatalf("continued=%v, Err() = %v; want the callback unwound with ErrComputedUnordered", continued.Load(), out.err())
	}
	failedWithUnorderedRead(t, read)
}

// An unordered Get inside a Group builder unwinds the builder: the run fails
// with the sentinel and the container's children never run.
func TestComputedGet_UnorderedBuilderFailsTheContainerWithTheSentinel(t *testing.T) {
	out := newQuietOutput(t, false)
	producer := Compute(out.Task("make"), func(context.Context) (int, error) { return 1, nil })
	var declared atomic.Bool
	builder := out.Group("builder")
	builder.define(func(g *Group) {
		_ = producer.Get()
		declared.Store(true)
		g.Task("child").Define(func(context.Context) error { return nil })
	})
	_ = out.finish()
	if !errors.Is(out.err(), ErrComputedUnordered) {
		t.Errorf("Err() = %v, want ErrComputedUnordered", out.err())
	}
	if declared.Load() {
		t.Error("builder continued past an unordered Get")
	}
	if err := builder.wait(); err == nil {
		t.Error("builder settled successfully, want it failed by the unordered read")
	}
}

// A callback that reads a Computed through a goroutine it started is still
// the reader: the goroutine is its hands. Nothing orders it after the
// producer, so the read is refused whether or not the producer happened to
// settle first, and the Task fails with the sentinel when the callback
// returns.
func TestComputedGet_UnorderedCallbackReadingThroughASpawnedGoroutineIsRefused(t *testing.T) {
	out := newQuietOutput(t, false)
	made := out.Group("producers").Task("make")
	producer := Compute(made, func(context.Context) (int, error) { return 1, nil })
	read := out.Group("consumers").Task("read")
	var got atomic.Int64
	read.Define(func(context.Context) error {
		_ = made.wait()
		done := make(chan struct{})
		go func() { defer close(done); got.Store(int64(producer.Get())) }()
		<-done
		return nil
	})
	_ = out.finish()
	if !errors.Is(out.err(), ErrComputedUnordered) {
		t.Errorf("Err() = %v, want ErrComputedUnordered", out.err())
	}
	if got.Load() != 0 {
		t.Errorf("Get() through the spawned goroutine = %d, want the zero value of a refused read", got.Load())
	}
	failedWithUnorderedRead(t, read)
}

// The callback's hands reach through any number of goroutines: a read from
// a goroutine started by a goroutine the callback started is refused the
// same.
func TestComputedGet_UnorderedCallbackReadingThroughAGrandchildGoroutineIsRefused(t *testing.T) {
	out := newQuietOutput(t, false)
	made := out.Group("producers").Task("make")
	producer := Compute(made, func(context.Context) (int, error) { return 1, nil })
	read := out.Group("consumers").Task("read")
	read.Define(func(context.Context) error {
		_ = made.wait()
		outer := make(chan struct{})
		go func() {
			defer close(outer)
			inner := make(chan struct{})
			go func() { defer close(inner); _ = producer.Get() }()
			<-inner
		}()
		<-outer
		return nil
	})
	_ = out.finish()
	failedWithUnorderedRead(t, read)
}

// Strict mode, an unordered callback whose own child goroutine reads a
// Computed. The read is refused, and the goroutine that made it has no
// recover to take a strict panic, so the Task fails with
// ErrComputedUnordered instead of the whole process ending.
func TestStrictSpawnedUnorderedReadFailsTheTaskInsteadOfEndingTheProcess(t *testing.T) {
	if os.Getenv("EVO_STRICT_SPAWNED_CHILD") == "1" {
		out := initRun(Config{Output: Plain, Facades: Facades{Stderr: io.Discard}}, tuning{strict: true})
		made := out.Group("producers").Task("make")
		producer := Compute(made, func(context.Context) (int, error) { return 1, nil })
		_ = made.wait()
		read := out.Group("consumers").Task("read")
		read.Define(func(context.Context) error {
			done := make(chan struct{})
			go func() { defer close(done); _ = producer.Get() }()
			<-done
			return nil
		})
		_ = read.wait()
		if snap := read.snapshot(); snap.State == failed && strings.Contains(snap.Summary, ErrComputedUnordered.Error()) {
			os.Exit(0)
		}
		os.Exit(3)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStrictSpawnedUnorderedReadFailsTheTaskInsteadOfEndingTheProcess$")
	cmd.Env = append(os.Environ(), "EVO_STRICT_SPAWNED_CHILD=1")
	b, err := cmd.CombinedOutput()
	if err != nil {
		first, _, _ := strings.Cut(string(b), "\n")
		t.Fatalf("child process: %v; first line: %s", err, first)
	}
}
