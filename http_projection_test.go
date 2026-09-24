package evo_test

// Spec §53 (ZYS-946): CLI and HTTP use the same model. An embedder builds
// one Isolated, FormatExternal Output per request, drives it with
// Output.Run(r.Context(), ...), and serializes the Result with WriteJSON.
// These tests pin the lifecycle, isolation, and error semantics that
// contract depends on.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// runDoc is the subset of the "evo.run" wire document these tests read.
type runDoc struct {
	RunID    string `json:"run_id"`
	Outcome  string `json:"outcome"`
	ExitCode int    `json:"exit_code"`
	Data     struct {
		Tasks []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"tasks"`
	} `json:"data"`
}

func decodeRunDoc(t *testing.T, body []byte) runDoc {
	t.Helper()
	var doc runDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decode evo.run document: %v\n%s", err, body)
	}
	return doc
}

func (d runDoc) taskState(name string) string {
	for _, task := range d.Data.Tasks {
		if task.Name == name {
			return task.State
		}
	}
	return ""
}

// embedderOutput is the §53 per-request configuration.
func embedderOutput() *evo.Output {
	return evo.Init(evo.Config{
		Isolated: true,
		Format:   evo.FormatExternal,
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	})
}

// One runtime truth: the document FormatJSON writes to Stdout at the end of
// a run and the document WriteJSON produces from that same run's Result are
// the same bytes — the HTTP projection cannot drift from the CLI one.
func TestWriteJSON_MatchesFormatJSONDocumentForSameRun(t *testing.T) {
	var stdout bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Format: evo.FormatJSON, Stdout: &stdout, Stderr: io.Discard})
	result := out.Run(context.Background(), func(ctx context.Context) error {
		seq := out.Sequence("launch agent")
		seq.Task("register").Define(func(context.Context) error { return nil })
		seq.Task("start").Define(func(ctx context.Context) error {
			return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectCreate, Object: "service", Quantity: 1},
				func(context.Context) error { return nil })
		})
		return nil
	})

	var written bytes.Buffer
	if err := evo.WriteJSON(&written, result); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if stdout.String() != written.String() {
		t.Fatalf("WriteJSON drifted from FormatJSON's own document\nFormatJSON:\n%s\nWriteJSON:\n%s", stdout.String(), written.String())
	}
	if !strings.HasSuffix(written.String(), "}\n") || strings.HasSuffix(written.String(), "\n\n") {
		t.Fatalf("WriteJSON must end with exactly one trailing newline, got %q", tail(written.String()))
	}
}

func tail(s string) string {
	const n = 8
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// errClientGone stands in for a transport failure such as a reset
// connection.
var errClientGone = errors.New("client gone")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errClientGone }

// Transport failure is the embedder's to classify: WriteJSON names what it
// was doing and keeps the writer's error reachable through errors.Is.
func TestWriteJSON_WriterFailureNamesOperationAndKeepsCause(t *testing.T) {
	out := embedderOutput()
	result := out.Run(context.Background(), func(context.Context) error { return nil })

	err := evo.WriteJSON(failingWriter{}, result)
	if !errors.Is(err, errClientGone) {
		t.Fatalf("WriteJSON error = %v, want errors.Is(err, errClientGone)", err)
	}
	if !strings.Contains(err.Error(), "evo.run document") {
		t.Fatalf("WriteJSON error %q does not name the document it failed to write", err)
	}
}

// Request lifecycle: when the caller's context ends (client disconnect,
// handler deadline), the run concludes Cancelled with exit 130 — the same
// truth a ^C produces — and queued work is reported not started instead
// of running on after the request is gone.
func TestOutputRun_CallerContextEndConcludesCancelled(t *testing.T) {
	// Each case yields the caller's ctx and the trigger that ends it from
	// inside the running Task; a deadline ends on its own.
	cases := map[string]struct {
		makeCtx func() (ctx context.Context, trigger, cleanup func())
		cause   string
	}{
		"cancel": {
			makeCtx: func() (context.Context, func(), func()) {
				ctx, cancel := context.WithCancel(context.Background())
				return ctx, cancel, cancel
			},
			cause: "by caller",
		},
		"deadline": {
			makeCtx: func() (context.Context, func(), func()) {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
				return ctx, func() {}, cancel
			},
			cause: "deadline exceeded",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, trigger, cleanup := tc.makeCtx()
			defer cleanup()
			out := embedderOutput()
			result := out.Run(ctx, func(context.Context) error {
				seq := out.Sequence("launch agent")
				seq.Task("register").Define(func(ctx context.Context) error {
					trigger()
					<-ctx.Done()
					return ctx.Err()
				})
				seq.Task("start").Define(func(context.Context) error {
					t.Error("queued Task ran after the caller's context ended")
					return nil
				})
				return nil
			})

			if result.Conclusion.State != evo.StateCancelled || result.ExitCode() != evo.ExitCancelled {
				t.Fatalf("conclusion = %s/%d, want %s/%d", result.Conclusion.State, result.ExitCode(), evo.StateCancelled, evo.ExitCancelled)
			}
			if result.Conclusion.Explanation != tc.cause {
				t.Fatalf("cancellation cause = %q, want %q", result.Conclusion.Explanation, tc.cause)
			}
			var body bytes.Buffer
			if err := evo.WriteJSON(&body, result); err != nil {
				t.Fatalf("WriteJSON: %v", err)
			}
			doc := decodeRunDoc(t, body.Bytes())
			if doc.Outcome != "cancelled" || doc.ExitCode != evo.ExitCancelled {
				t.Fatalf("document outcome = %s/%d, want cancelled/130", doc.Outcome, doc.ExitCode)
			}
			if got := doc.taskState("start"); got != string(evo.NotStarted) {
				t.Fatalf("queued Task state = %q, want %q", got, evo.NotStarted)
			}
		})
	}
}

// concurrentRequests is enough overlapping runs to expose shared state
// under -race without slowing the suite.
const concurrentRequests = 16

// Concurrent requests never share runtime state: each run's document holds
// only its own Tasks, carries its own run identity, and the package
// default Output never sees any of them.
func TestIsolatedOutputs_ConcurrentRunsKeepSeparateTruth(t *testing.T) {
	defaultBefore := len(evo.Default().Snapshot().Tasks)
	bodies := make([][]byte, concurrentRequests)
	var wg sync.WaitGroup
	for i := range concurrentRequests {
		wg.Go(func() {
			out := embedderOutput()
			result := out.Run(context.Background(), func(context.Context) error {
				out.Task(fmt.Sprintf("request %d", i)).Define(func(context.Context) error { return nil })
				return nil
			})
			var body bytes.Buffer
			if err := evo.WriteJSON(&body, result); err != nil {
				t.Errorf("request %d: WriteJSON: %v", i, err)
				return
			}
			bodies[i] = body.Bytes()
		})
	}
	wg.Wait()

	runIDs := make(map[string]int, concurrentRequests)
	for i, body := range bodies {
		doc := decodeRunDoc(t, body)
		if len(doc.Data.Tasks) != 1 || doc.Data.Tasks[0].Name != fmt.Sprintf("request %d", i) {
			t.Errorf("request %d document holds tasks %+v, want only its own", i, doc.Data.Tasks)
		}
		if prev, dup := runIDs[doc.RunID]; dup {
			t.Errorf("requests %d and %d share run_id %q", prev, i, doc.RunID)
		}
		runIDs[doc.RunID] = i
	}
	if after := len(evo.Default().Snapshot().Tasks); after != defaultBefore {
		t.Fatalf("package default gained %d Tasks from Isolated runs", after-defaultBefore)
	}
}

// preCancelledProbeRuns is how many runs it takes to hit the window where a
// caller's already-ended ctx interrupts before the run context exists; the
// original report hung at run 123 of 2000.
const preCancelledProbeRuns = 2000

// preCancelledProbeBudget bounds the whole probe: every run returns at
// once, so exceeding it means one run's context was never cancelled.
const preCancelledProbeBudget = 30 * time.Second

// A request whose client disconnected before the handler reached Run still
// concludes: the interrupt the ended ctx triggers must cancel the context
// the run body actually waits on, never a placeholder replaced after it.
func TestOutputRun_PreCancelledCallerContextNeverHangs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan evo.Result, 1)
	go func() {
		var last evo.Result
		for range preCancelledProbeRuns {
			last = embedderOutput().Run(ctx, func(rc context.Context) error {
				<-rc.Done()
				return rc.Err()
			})
			if last.ExitCode() != evo.ExitCancelled {
				break
			}
		}
		done <- last
	}()
	select {
	case result := <-done:
		if result.Conclusion.State != evo.StateCancelled || result.ExitCode() != evo.ExitCancelled {
			t.Fatalf("pre-cancelled run concluded %s/%d, want %s/%d", result.Conclusion.State, result.ExitCode(), evo.StateCancelled, evo.ExitCancelled)
		}
	case <-time.After(preCancelledProbeBudget):
		t.Fatal("a run on a pre-cancelled caller ctx hung: its run context was installed after the interrupt and never cancelled")
	}
}

// firstTaskID is the wire id 1.1 gave a run's first Task: the id sequence
// spent 1 on the run itself. A random run_id must not renumber every
// Task, Group, and message a consumer already correlates on.
const firstTaskID = "task_2"

func TestRunDocument_TaskIDsKeepTheirNumbering(t *testing.T) {
	var stdout bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Format: evo.FormatJSON, Stdout: &stdout, Stderr: io.Discard})
	out.Run(context.Background(), func(context.Context) error {
		out.Task("register").Define(func(context.Context) error { return nil })
		return nil
	})
	doc := decodeRunDoc(t, stdout.Bytes())
	if len(doc.Data.Tasks) != 1 || doc.Data.Tasks[0].ID != firstTaskID {
		t.Fatalf("tasks = %+v, want one Task with id %q", doc.Data.Tasks, firstTaskID)
	}
}
