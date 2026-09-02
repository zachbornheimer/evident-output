package evo_test

import (
	"errors"
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestEach_DrivesAbsoluteProgressAndPhase(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("install")
	packages := []string{"alpha", "beta", "gamma"}

	var seenPhases []string
	var seenCompleted []int64
	for pkg := range task.Each(packages) {
		snap := task.Snapshot()
		seenPhases = append(seenPhases, snap.Phase)
		seenCompleted = append(seenCompleted, snap.Progress.Completed)
		if snap.Progress.Total != int64(len(packages)) {
			t.Fatalf("total = %d, want %d", snap.Progress.Total, len(packages))
		}
		_ = pkg
	}

	if want := []string{"alpha", "beta", "gamma"}; !equalStrings(seenPhases, want) {
		t.Fatalf("phases = %v, want %v", seenPhases, want)
	}
	if want := []int64{1, 2, 3}; !equalInt64s(seenCompleted, want) {
		t.Fatalf("completed = %v, want %v", seenCompleted, want)
	}
	final := task.Snapshot()
	if final.Progress.Completed != 3 || final.Progress.Total != 3 {
		t.Fatalf("final progress = %#v, want 3/3", final.Progress)
	}
}

func TestEach_BreakLeavesProgressAtCompletedCount(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("install")
	packages := []string{"alpha", "beta", "gamma", "delta"}

	for pkg := range task.Each(packages) {
		if pkg == "beta" {
			break
		}
	}

	snap := task.Snapshot()
	if snap.Progress.Completed != 2 || snap.Progress.Total != 4 {
		t.Fatalf("progress = %#v, want 2/4 (early break leaves count as-is)", snap.Progress)
	}
	if snap.State == evo.Done {
		t.Fatal("break must not auto-resolve the task")
	}
}

func TestEach_RetryInsideBodyDoesNotAdvance(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("install")
	packages := []string{"alpha", "beta"}

	for pkg := range task.Each(packages) {
		before := task.Snapshot().Progress.Completed
		// Simulate a caller retrying work for the same item without ever
		// calling task.Progress/Phase itself.
		attempt := func() { _ = pkg }
		attempt()
		attempt()
		attempt()
		after := task.Snapshot().Progress.Completed
		if before != after {
			t.Fatalf("retrying inside the loop body moved progress: %d -> %d", before, after)
		}
	}

	if got := task.Snapshot().Progress.Completed; got != 2 {
		t.Fatalf("completed = %d, want 2", got)
	}
}

func TestEachN_DrivesProgressOnly(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("scan")

	var seenCompleted []int64
	for i := range task.EachN(3) {
		seenCompleted = append(seenCompleted, task.Snapshot().Progress.Completed)
		_ = i
	}

	if want := []int64{1, 2, 3}; !equalInt64s(seenCompleted, want) {
		t.Fatalf("completed = %v, want %v", seenCompleted, want)
	}
	if task.Snapshot().Phase != "" {
		t.Fatalf("EachN must not set Phase, got %q", task.Snapshot().Phase)
	}
}

func TestProgress_SealedTotalChangeRecordsMisuse(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("t")
	task.Progress(14, 40)
	task.Progress(14, 53) // discovery grows the denominator after it sealed

	snap := task.Snapshot()
	if snap.Progress.Total != 40 {
		t.Fatalf("sealed total was not preserved: %#v", snap.Progress)
	}
	if !errors.Is(out.Err(), evo.ErrInvalidProgress) {
		t.Fatalf("error = %v, want ErrInvalidProgress", out.Err())
	}
}

func TestProgress_IndeterminateToDeterminateOnce(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("t")
	task.Phase("scanning")
	if got := task.Snapshot().Progress.Kind; got != evo.Indeterminate {
		t.Fatalf("kind = %q, want Indeterminate before the denominator is known", got)
	}

	task.Progress(5, 10) // the one allowed indeterminate -> determinate transition
	if got := task.Snapshot().Progress.Kind; got != evo.Determinate {
		t.Fatalf("kind = %q, want Determinate", got)
	}

	task.Progress(6, 10) // same sealed total: an ordinary update, not a new transition
	if snap := task.Snapshot(); snap.Progress.Completed != 6 || snap.Progress.Total != 10 {
		t.Fatalf("progress = %#v, want 6/10", snap.Progress)
	}

	task.Progress(7, 20) // attempting to reseal with a different total is misuse
	snap := task.Snapshot()
	if snap.Progress.Total != 10 || snap.Progress.Completed != 6 {
		t.Fatalf("progress = %#v, want unchanged 6/10", snap.Progress)
	}
	if !errors.Is(out.Err(), evo.ErrInvalidProgress) {
		t.Fatalf("error = %v, want ErrInvalidProgress", out.Err())
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalInt64s(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
