package evo_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// The §39 callback-entry rate measures Define callbacks Evo decided on:
// entered, or proven current and skipped. Work that never reached that
// decision (a failed predecessor cascaded it NotStarted) is neither, so it
// must not read as a skip.

const cascadedDependents = 10

func TestMetrics_CallbackEntryRateIgnoresWorkAFailedPredecessorNeverReleased(t *testing.T) {
	t.Parallel()
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	failing := out.Group("setup")
	failing.Task("prepare").Define(func(context.Context) error { return errors.New("prepare failed") })
	for i := range cascadedDependents {
		out.Task(fmt.Sprintf("dependent %d", i)).After(failing).Define(func(context.Context) error { return nil })
	}
	_ = out.Finish()

	m := out.Conclusion().Metrics()
	if m.Defined != 1 || m.Entered != 1 {
		t.Fatalf("Defined = %d, Entered = %d; want 1 and 1: the %d cascaded dependents never reached a callback decision", m.Defined, m.Entered, cascadedDependents)
	}
	if got := m.CallbackEntryRate(); got != 1 {
		t.Fatalf("CallbackEntryRate = %v, want 1: nothing was proven current and skipped", got)
	}
}

func TestMetrics_CallbackEntryRateCountsAVerifySkipAsDecided(t *testing.T) {
	t.Parallel()
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	current := out.Task("current")
	current.Verify(func(context.Context) (bool, error) { return true, nil })
	current.Define(func(context.Context) error { return nil })
	out.Task("stale").Define(func(context.Context) error { return nil })
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	m := out.Conclusion().Metrics()
	if m.Defined != 2 || m.Entered != 1 || m.CallbackEntryRate() != 0.5 {
		t.Fatalf("Defined = %d, Entered = %d, rate = %v; want 2, 1, 0.5", m.Defined, m.Entered, m.CallbackEntryRate())
	}
}
