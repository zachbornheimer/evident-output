package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// testBudget is generous: only the budget test expects it to run out.
const testBudget = 10 * time.Second

func newServer(t *testing.T, a agent, stateDir string, budget time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(runHandler{
		agent: a, stateDir: stateDir, budget: budget,
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	t.Cleanup(srv.Close)
	return srv
}

func postLaunch(ctx context.Context, t *testing.T, srv *httptest.Server) (int, []byte, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		return 0, nil, err
	}
	body, err := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); err == nil {
		err = closeErr
	}
	return resp.StatusCode, body, err
}

// volatileKeys differ between any two runs: identity, wall clock, timing.
var volatileKeys = map[string]bool{
	"run_id": true, "started_at": true, "finished_at": true, "duration_ms": true, "timing": true,
}

// comparable decodes an evo.run document with its volatile fields removed
// and stateDir replaced, so two runs of one model compare equal.
func comparable(t *testing.T, body []byte, stateDir string) any {
	t.Helper()
	var doc any
	if err := json.Unmarshal(bytes.ReplaceAll(body, []byte(stateDir), []byte("<state>")), &doc); err != nil {
		t.Fatalf("decode evo.run document: %v\n%s", err, body)
	}
	return stripVolatile(doc)
}

func stripVolatile(v any) any {
	switch node := v.(type) {
	case map[string]any:
		for key, child := range node {
			if volatileKeys[key] {
				delete(node, key)
				continue
			}
			node[key] = stripVolatile(child)
		}
	case []any:
		for i, child := range node {
			node[i] = stripVolatile(child)
		}
	}
	return v
}

// Spec §53: the HTTP answer is the document the CLI's FormatJSON prints
// for the same model — one runtime truth, two transports.
func TestLaunchHTTP_AnswersWithTheCLIDocument(t *testing.T) {
	cliDir, httpDir := t.TempDir(), t.TempDir()

	var cli bytes.Buffer
	out := evo.Init(evo.Config{Title: "launch agent", Format: evo.FormatJSON, StateDir: cliDir, Isolated: true, Stdout: &cli, Stderr: io.Discard})
	out.Run(context.Background(), func(context.Context) error {
		launchAgent(out, newAgent(cliDir))
		return nil
	})

	status, body, err := postLaunch(context.Background(), t, newServer(t, newAgent(httpDir), httpDir, testBudget))
	if err != nil || status != http.StatusOK {
		t.Fatalf("POST /launch = %d, %v\n%s", status, err, body)
	}
	if got, want := comparable(t, body, httpDir), comparable(t, cli.Bytes(), cliDir); !reflect.DeepEqual(got, want) {
		t.Fatalf("HTTP document drifted from the CLI's\nHTTP: %s\nCLI:  %s", body, cli.Bytes())
	}
}

type launchDoc struct {
	RunID   string `json:"run_id"`
	Outcome string `json:"outcome"`
	Data    struct {
		Tasks []struct {
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"tasks"`
		Effects []struct {
			Subject string `json:"subject"`
			Status  string `json:"status"`
		} `json:"effects"`
	} `json:"data"`
}

func decodeLaunch(t *testing.T, body []byte) launchDoc {
	t.Helper()
	var doc launchDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decode evo.run document: %v\n%s", err, body)
	}
	return doc
}

func (d launchDoc) changed(subject string) bool {
	for _, e := range d.Data.Effects {
		if e.Subject == subject && e.Status == "changed" {
			return true
		}
	}
	return false
}

func (d launchDoc) state(task string) string {
	for _, tk := range d.Data.Tasks {
		if tk.Name == task {
			return tk.State
		}
	}
	return ""
}

// concurrentLaunches overlaps enough requests to expose shared state.
const concurrentLaunches = 8

// Concurrent requests each get their own run: their own identity, their
// own two Tasks, and a success answer.
func TestLaunchHTTP_ConcurrentRequestsGetSeparateRuns(t *testing.T) {
	dir := t.TempDir()
	srv := newServer(t, newAgent(dir), dir, testBudget)

	bodies := make([][]byte, concurrentLaunches)
	var wg sync.WaitGroup
	for i := range concurrentLaunches {
		wg.Go(func() {
			status, body, err := postLaunch(context.Background(), t, srv)
			if err != nil || status != http.StatusOK {
				t.Errorf("request %d: POST /launch = %d, %v\n%s", i, status, err, body)
			}
			bodies[i] = body
		})
	}
	wg.Wait()

	seen := make(map[string]bool, concurrentLaunches)
	for i, body := range bodies {
		doc := decodeLaunch(t, body)
		if len(doc.Data.Tasks) != 2 || doc.state("write plist") != "done" || doc.state("load agent") != "done" {
			t.Errorf("request %d tasks = %+v, want its own write plist + load agent, both done", i, doc.Data.Tasks)
		}
		if seen[doc.RunID] {
			t.Errorf("request %d reused run_id %q", i, doc.RunID)
		}
		seen[doc.RunID] = true
	}
}

// loadProbe reports when a blockingAgent's load starts and when it saw
// its context end.
type loadProbe struct {
	started <-chan struct{}
	ended   <-chan struct{}
}

// blockingAgent's load waits for its context to end — a launchctl call
// that outlives the request.
func blockingAgent(dir string) (agent, loadProbe) {
	a := newAgent(dir)
	started, ended := make(chan struct{}), make(chan struct{})
	a.load = func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(ended)
		return ctx.Err()
	}
	return a, loadProbe{started: started, ended: ended}
}

// shortBudget runs out inside the blocking load, while leaving the plist
// write ample time to commit first.
const shortBudget = time.Second

// An exhausted budget answers 503 with a Cancelled document that still
// reports the plist write already committed.
func TestLaunchHTTP_ExhaustedBudgetAnswersCancelledWithCommittedWork(t *testing.T) {
	dir := t.TempDir()
	a, _ := blockingAgent(dir)
	status, body, err := postLaunch(context.Background(), t, newServer(t, a, dir, shortBudget))
	if err != nil || status != http.StatusServiceUnavailable {
		t.Fatalf("POST /launch = %d, %v, want 503\n%s", status, err, body)
	}
	doc := decodeLaunch(t, body)
	if doc.Outcome != "cancelled" || doc.state("write plist") != "done" || doc.state("load agent") != "cancelled" {
		t.Fatalf("outcome %q, tasks %+v; want cancelled with write plist done and load agent cancelled\n%s", doc.Outcome, doc.Data.Tasks, body)
	}
	if !doc.changed("write plist") {
		t.Fatalf("committed plist Effect missing from the cancelled document\n%s", body)
	}
}

// A client that disconnects cancels its run: the in-flight load observes
// its context ending instead of running on for nobody.
func TestLaunchHTTP_ClientDisconnectCancelsTheRun(t *testing.T) {
	dir := t.TempDir()
	a, load := blockingAgent(dir)
	srv := newServer(t, a, dir, testBudget)

	ctx, disconnect := context.WithCancel(context.Background())
	defer disconnect()
	go func() {
		<-load.started
		disconnect()
	}()
	if _, _, err := postLaunch(ctx, t, srv); err == nil {
		t.Fatal("request completed although the load blocks until cancelled")
	}
	select {
	case <-load.ended:
	case <-time.After(testBudget / 2):
		t.Fatal("client disconnect never reached the running load")
	}
}

func TestStatusFor_MapsConclusionState(t *testing.T) {
	cases := map[evo.ConclusionState]int{
		evo.StateReady:     http.StatusOK,
		evo.StateBlocked:   http.StatusConflict,
		evo.StateFailed:    http.StatusInternalServerError,
		evo.StateCancelled: http.StatusServiceUnavailable,
	}
	for state, want := range cases {
		if got := statusFor(state); got != want {
			t.Errorf("statusFor(%s) = %d, want %d", state, got, want)
		}
	}
}

// serve shuts down cleanly when its run's context ends — the path
// evo.Main takes on SIGINT/SIGTERM.
func TestServe_ContextEndShutsDownCleanly(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	o := options{stateDir: dir, serve: "127.0.0.1:0", budget: testBudget}
	if err := serve(ctx, o, newAgent(dir)); err != nil {
		t.Fatalf("serve after context end = %v, want clean shutdown", err)
	}
}

// lockWaitDeadline bounds how long a queued request may take to answer
// once its budget ran out while it waited on the state lock.
const lockWaitDeadline = 5 * shortBudget

// Every request shares one StateDir, so a request queues on the exclusive
// per-manifest lock (spec §11.3) while another run holds it — and its
// budget keeps running down while it waits. When the budget runs out in
// the queue, the request answers 503 Cancelled at once; it neither waits
// for the holder to finish nor reports work it never started.
func TestLaunchHTTP_BudgetRunsOutWhileQueuedOnStateLock(t *testing.T) {
	dir := t.TempDir()
	holder, load := blockingAgent(dir)
	holderCtx, releaseHolder := context.WithCancel(context.Background())
	defer releaseHolder()
	holderDone := make(chan struct{})
	go func() {
		defer close(holderDone)
		_, _, _ = postLaunch(holderCtx, t, newServer(t, holder, dir, testBudget))
	}()
	<-load.started

	answered := make(chan struct{})
	var (
		status int
		body   []byte
		err    error
	)
	go func() {
		defer close(answered)
		status, body, err = postLaunch(context.Background(), t, newServer(t, newAgent(dir), dir, shortBudget))
	}()
	select {
	case <-answered:
	case <-time.After(lockWaitDeadline):
		releaseHolder()
		<-holderDone
		t.Fatal("a request whose budget ran out while queued on the state lock never answered")
	}
	releaseHolder()
	<-holderDone

	if err != nil || status != http.StatusServiceUnavailable {
		t.Fatalf("queued POST /launch = %d, %v, want 503\n%s", status, err, body)
	}
	doc := decodeLaunch(t, body)
	if doc.Outcome != "cancelled" || doc.state("write plist") != "cancelled" || doc.state("load agent") != string(evo.NotStarted) {
		t.Fatalf("outcome %q, tasks %+v; want cancelled with write plist cancelled and load agent not started\n%s", doc.Outcome, doc.Data.Tasks, body)
	}
}
