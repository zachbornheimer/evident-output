package evo_test

// Spec §53 (ZYS-946): CLI and HTTP use the same model. An embedder builds
// one Isolated, FormatExternal Output per request, drives it with
// Output.Run(r.Context(), ...), and serializes the Result with WriteJSON —
// all 1.1 API. These tests pin the isolation and error semantics that
// contract depends on; the 1.1 lifecycle itself is pinned in
// caller_context_test.go.

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

	evo "github.com/zachbornheimer/evident-output"
)

// runDoc is the subset of the "evo.run" wire document these tests read.
type runDoc struct {
	Data struct {
		Tasks []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
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
// Both already matched in 1.1; this guards the shared writer against
// future drift. TestFormatJSON_WriterFailureKeeps11Identity and
// TestWriteJSON_WriterFailureReturnsTheWriterError pin that sharing it
// changed neither error.
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

// Error identity is behavior (ZYS-946 owner rule): WriteJSON returns the
// writer's error itself, as 1.1 did, so a host comparing err ==
// syscall.EPIPE (or its own sentinel), or matching err.Error(), keeps
// working on upgrade.
func TestWriteJSON_WriterFailureReturnsTheWriterError(t *testing.T) {
	out := embedderOutput()
	result := out.Run(context.Background(), func(context.Context) error { return nil })

	if err := evo.WriteJSON(failingWriter{}, result); err != errClientGone {
		t.Fatalf("WriteJSON error = %#v, want the writer's error itself (%#v), as in 1.1", err, errClientGone)
	}
}

// concurrentRequests is enough overlapping runs to expose shared state
// under -race without slowing the suite.
const concurrentRequests = 16

// Concurrent requests never share runtime state: each run's document holds
// only its own Tasks, and the package default Output never sees any of
// them. (Every run still carries the 1.1 run_id "out_1"; a per-run
// identity is deferred behind ZYS-947.)
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

	for i, body := range bodies {
		doc := decodeRunDoc(t, body)
		if len(doc.Data.Tasks) != 1 || doc.Data.Tasks[0].Name != fmt.Sprintf("request %d", i) {
			t.Errorf("request %d document holds tasks %+v, want only its own", i, doc.Data.Tasks)
		}
	}
	if after := len(evo.Default().Snapshot().Tasks); after != defaultBefore {
		t.Fatalf("package default gained %d Tasks from Isolated runs", after-defaultBefore)
	}
}

// firstTaskID is the wire id 1.1 gave a run's first Task: the id sequence
// spent 1 on the run itself. 1.2 must not renumber any Task, Group, or
// message a consumer already correlates on.
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

// FormatJSON's end-of-run write failure keeps its 1.1 identity: exactly
// one wrap of ErrRenderer carrying the writer error's text, not the
// writer's error itself. A host that branches on errors.Is(err,
// ErrRenderer) before any transport check sees the same branch as in 1.1.
func TestFormatJSON_WriterFailureKeeps11Identity(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Format: evo.FormatJSON, Stdout: failingWriter{}, Stderr: io.Discard})
	out.Task("register").Define(func(context.Context) error { return nil })

	err := out.Finish()
	if errors.Unwrap(err) != evo.ErrRenderer {
		t.Fatalf("Finish error = %#v, want a single wrap of evo.ErrRenderer (1.1)", err)
	}
	if errors.Is(err, errClientGone) {
		t.Fatalf("Finish error %v matches the writer's error; 1.1 carried only its text", err)
	}
	if want := evo.ErrRenderer.Error() + ": " + errClientGone.Error(); err.Error() != want {
		t.Fatalf("Finish error text = %q, want the 1.1 text %q", err.Error(), want)
	}
}
