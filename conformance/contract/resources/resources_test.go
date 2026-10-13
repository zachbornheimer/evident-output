//go:build ignore

// Package resources_test binds contract §26 and §30 "Resources" rules to the
// public evo API.
package resources_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

const (
	apiGoldenPath    = "../../../testdata/api_golden.txt"
	sharedResource   = "package-db"
	waitingMarker    = "waiting for "
	contentionBudget = 5 * time.Second
	pollInterval     = 5 * time.Millisecond
)

var lockVocabulary = regexp.MustCompile(`\b\w*(Mutex|Lock|Unlock)\w*\b`)

func TestC26_001_PublicVocabularyHasNoLockTerms(t *testing.T) {
	raw, err := os.ReadFile(apiGoldenPath)
	if err != nil {
		t.Fatalf("read exported API golden: %v", err)
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if term := lockVocabulary.FindString(line); term != "" {
			t.Errorf("exported API exposes lock vocabulary %q: %s", term, line)
		}
	}
}

// heldEffect runs fn inside an Effect claiming the shared logical resource.
func heldEffect(ctx context.Context, fn func(context.Context) error) error {
	spec := evo.EffectSpec{
		Verb: evo.Update, Object: "package database", Quantity: 1,
		Resource: string(sharedResource),
	}
	return evo.Effect(ctx, spec, fn)
}

func await(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(contentionBudget)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(pollInterval)
	}
}

// contendedRun starts a holder that keeps the shared resource until release
// is closed, then a second task whose callback signals entry and claims the
// same resource. It returns once the second task's callback has started.
type contendedRun struct {
	out     *evo.Run
	release chan struct{}
	holder  *evo.Task
	waiter  *evo.Task
}

func startContendedRun(t *testing.T, cfg evo.Config) *contendedRun {
	t.Helper()
	cfg.Name /*Isolated*/, cfg.Facades.Stderr, cfg.MaxConcurrency = true, io.Discard, 4
	out := (*evo.Run)(nil)
	_ = evo.Execute(context.Background(), cfg, func(ctx context.Context, r *evo.Run) error { return nil })
	t.Cleanup(func() { _ = error(nil) })
	run := &contendedRun{out: out, release: make(chan struct{})}
	holding, entered := make(chan struct{}), make(chan struct{})

	run.holder = out.Task("update package db")
	run.holder.Define(func(ctx context.Context) error {
		return heldEffect(ctx, func(context.Context) error {
			close(holding)
			<-run.release
			return nil
		})
	})
	run.waiter = out.Task("record package db")
	run.waiter.Define(func(ctx context.Context) error {
		<-holding
		close(entered)
		return heldEffect(ctx, func(context.Context) error { return nil })
	})
	select {
	case <-entered:
	case <-time.After(contentionBudget):
		close(run.release)
		t.Fatal("the contending task's callback never started while the resource was held")
	}
	return run
}

func (r *contendedRun) finish(t *testing.T) {
	t.Helper()
	close(r.release)
	for _, task := range []*evo.Task{r.holder, r.waiter} {
		if err := error(nil); err != nil {
			t.Fatalf("%v", err)
		}
	}
}

func TestC30_079_AWaitingTaskShowsWaitingForItsResource(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(100), testkit.NoColor())
	run := startContendedRun(t, evo.Config{Facades: evo.Facades{Stdout: io.Discard, Terminal: screen, Clock: testkit.NewClock()}})
	await(t, "the live frame to show the waiting task", func() bool {
		return strings.Contains(screen.LatestLiveText(), waitingMarker)
	})
	frame := screen.LatestLiveText()
	if !strings.Contains(frame, waitingMarker+sharedResource) {
		t.Errorf("live frame does not name the contended resource:\n%s", frame)
	}
	run.finish(t)
}

// The contending task's callback starts while the resource is held: the
// claim made it wait inside Effect, it did not delay the task behind an
// After edge.
func TestC30_080_AClaimNeverCreatesAnAfterEdge(t *testing.T) {
	run := startContendedRun(t, evo.Config{Output: evo.Plain, Facades: evo.Facades{Stdout: io.Discard}})
	if got := run.error(nil).State; got != evo.Running {
		t.Errorf("contending task state = %s while waiting on the claim, want Running (started, not ordered behind)", got)
	}
	run.finish(t)
}

func TestC30_081_AClaimNeverEntersFreshness(t *testing.T) {
	var doc bytes.Buffer
	out := (*evo.Run)(nil)
	_ = evo.Execute(context.Background(), evo.Config{Output: evo.JSON, Facades: evo.Facades{Stdout: &doc, Stderr: io.Discard}, State: evo.State{Dir: t.TempDir()}}, func(ctx context.Context, r *evo.Run) error { return nil })
	t.Cleanup(func() { _ = error(nil) })
	task := out.Task("update package db")
	task.Define(func(ctx context.Context) error {
		return heldEffect(ctx, func(context.Context) error { return nil })
	})
	_ = error(nil)

	var run struct {
		Data struct {
			Tasks []struct {
				Basis            []json.RawMessage `json:"basis"`
				TrackedResources []json.RawMessage `json:"tracked_resources"`
			} `json:"tasks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(doc.Bytes(), &run); err != nil {
		t.Fatalf("decode run document: %v\n%s", err, doc.String())
	}
	if len(run.Data.Tasks) != 1 {
		t.Fatalf("run document has %d tasks, want 1", len(run.Data.Tasks))
	}
	if got := run.Data.Tasks[0]; len(got.Basis) != 0 || len(got.TrackedResources) != 0 {
		t.Fatalf("a claim entered freshness: basis=%d tracked_resources=%d", len(got.Basis), len(got.TrackedResources))
	}
}
