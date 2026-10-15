package evo_test

import (
	"context"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

const runSummaryText = "release 1.4"

func summarisedRun(ctx context.Context, t *testing.T, mode evo.Output, summarise bool) contractRun {
	t.Helper()
	return executeContract(ctx, t, evo.Config{Name: "summary", Output: mode},
		func(_ context.Context, r *evo.Run) error {
			if summarise {
				r.Summary(runSummaryText)
			}
			r.Task("build").Define(func(context.Context) error { return nil })
			return nil
		})
}

// TestRunSummaryIsInTheRunDocument pins CONTRACT.md §8: the text Run.Summary
// prints in Human and Plain is the run's own `summary` in evo.run and in the
// payload of evo.event's run.finished, and a run that never called it carries
// no summary key in either.
func TestRunSummaryIsInTheRunDocument(t *testing.T) {
	ctx := context.Background()

	t.Run("evo.run carries it", func(t *testing.T) {
		doc := runDocument(t, summarisedRun(ctx, t, evo.JSON, true).Stdout)
		if doc["summary"] != runSummaryText {
			t.Errorf("evo.run summary = %v, want %q", doc["summary"], runSummaryText)
		}
	})

	t.Run("evo.event run.finished carries it", func(t *testing.T) {
		finished := eventsOfType(runEvents(t, summarisedRun(ctx, t, evo.JSONL, true).Stdout), "run.finished")
		if len(finished) != 1 {
			t.Fatalf("run.finished events = %d, want 1", len(finished))
		}
		if got := payload(finished[0])["summary"]; got != runSummaryText {
			t.Errorf("run.finished summary = %v, want %q", got, runSummaryText)
		}
	})

	t.Run("a run without one carries no summary key", func(t *testing.T) {
		if _, has := runDocument(t, summarisedRun(ctx, t, evo.JSON, false).Stdout)["summary"]; has {
			t.Error("evo.run has a summary key though Run.Summary was never called")
		}
		finished := eventsOfType(runEvents(t, summarisedRun(ctx, t, evo.JSONL, false).Stdout), "run.finished")
		if _, has := payload(finished[0])["summary"]; has {
			t.Error("run.finished has a summary key though Run.Summary was never called")
		}
	})
}
