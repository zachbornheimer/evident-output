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
		Verb: evo.EffectUpdate, Object: "package database", Quantity: 1,
		Resource: evo.LogicalResource(sharedResource),
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
	out     *evo.Output
	release chan struct{}
	holder  *evo.TaskHandle
	waiter  *evo.TaskHandle
}

func startContendedRun(t *testing.T, cfg evo.Config) *contendedRun {
	t.Helper()
	cfg.Isolated, cfg.Stderr, cfg.MaxConcurrency = true, io.Discard, 4
	out := evo.Init(cfg)
	t.Cleanup(func() { _ = out.Close() })
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
	for _, task := range []*evo.TaskHandle{r.holder, r.waiter} {
		if err := task.Wait(); err != nil {
			t.Fatalf("%v", err)
		}
	}
}

func TestC30_079_AWaitingTaskShowsWaitingForItsResource(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(100), testkit.NoColor())
	run := startContendedRun(t, evo.Config{
		Stdout: io.Discard, Terminal: screen, Color: evo.ColorNever,
		Clock: testkit.NewClock(), VisibilityDelay: evo.Delay(0),
	})
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
	run := startContendedRun(t, evo.Config{Stdout: io.Discard, Plain: true, Color: evo.ColorNever})
	if got := run.waiter.Snapshot().State; got != evo.Running {
		t.Errorf("contending task state = %s while waiting on the claim, want Running (started, not ordered behind)", got)
	}
	run.finish(t)
}

func TestC30_081_AClaimNeverEntersFreshness(t *testing.T) {
	var doc bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Format: evo.FormatJSON, Stdout: &doc, Stderr: io.Discard,
		StateDir: t.TempDir(),
	})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("update package db")
	task.Define(func(ctx context.Context) error {
		return heldEffect(ctx, func(context.Context) error { return nil })
	})
	_ = out.Finish()

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
