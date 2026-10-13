package evo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// The helpers below drive package evo only through its exported surface, the
// way a program written from CONTRACT.md would. Contract tests use them so a
// red test names a broken promise, not a broken internal.

// contractOutputs is every Output mode CONTRACT.md §6 lists.
var contractOutputs = []evo.Output{evo.Human, evo.Plain, evo.JSON, evo.JSONL}

// outputName is the EVO_OUTPUT spelling of each mode, for subtest names.
var outputName = map[evo.Output]string{
	evo.Human: "human",
	evo.Plain: "plain",
	evo.JSON:  "json",
	evo.JSONL: "jsonl",
}

// contractRunDeadline bounds one Execute, so a run that never finishes fails
// its test instead of hanging the suite.
const contractRunDeadline = 30 * time.Second

// contractRun is one finished Execute: its Result and both streams.
type contractRun struct {
	Result evo.Result
	Stdout string
	Stderr string
}

// Text is everything a person would read: both streams, in order.
func (c contractRun) Text() string { return c.Stdout + c.Stderr }

// lockedBuffer is a bytes.Buffer safe for the concurrent writes a run makes.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// executeContract runs body under cfg with captured streams and an empty
// stdin, and fails the test if the run does not return by the deadline.
func executeContract(ctx context.Context, t *testing.T, cfg evo.Config, body evo.RunFunc) contractRun {
	t.Helper()
	var stdout, stderr lockedBuffer
	cfg.Facades.Stdout = &stdout
	cfg.Facades.Stderr = &stderr
	if cfg.Facades.Stdin == nil {
		cfg.Facades.Stdin = strings.NewReader("")
	}
	done := make(chan evo.Result, 1)
	go func() { done <- evo.Execute(ctx, cfg, body) }()
	select {
	case res := <-done:
		return contractRun{Result: res, Stdout: stdout.String(), Stderr: stderr.String()}
	case <-time.After(contractRunDeadline):
		t.Fatalf("Execute did not return within %s; output so far:\n%s%s",
			contractRunDeadline, stdout.String(), stderr.String())
		return contractRun{}
	}
}

// runDocument decodes the one evo.run document a JSON run writes to stdout.
func runDocument(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not one evo.run document: %v\n%s", err, stdout)
	}
	if doc["object"] != "evo.run" {
		t.Fatalf("stdout object = %v, want evo.run", doc["object"])
	}
	return doc
}

// runEvents decodes the evo.event lines a JSONL run writes to stdout.
func runEvents(t *testing.T, stdout string) []map[string]any {
	t.Helper()
	var events []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(stdout), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("stdout line is not an evo.event: %v\n%s", err, line)
		}
		if event["object"] != "evo.event" {
			t.Fatalf("stdout line object = %v, want evo.event", event["object"])
		}
		events = append(events, event)
	}
	return events
}

// eventsOfType keeps the events whose type is typ.
func eventsOfType(events []map[string]any, typ string) []map[string]any {
	var out []map[string]any
	for _, event := range events {
		if event["type"] == typ {
			out = append(out, event)
		}
	}
	return out
}

// documentTasks is the data.tasks list of an evo.run document.
func documentTasks(t *testing.T, doc map[string]any) []map[string]any {
	t.Helper()
	data, _ := doc["data"].(map[string]any)
	list, ok := data["tasks"].([]any)
	if !ok {
		t.Fatalf("evo.run has no data.tasks list: %v", doc["data"])
	}
	tasks := make([]map[string]any, 0, len(list))
	for _, item := range list {
		task, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("data.tasks holds a non-object: %v", item)
		}
		tasks = append(tasks, task)
	}
	return tasks
}

// payload is an event's payload object, or nil.
func payload(event map[string]any) map[string]any {
	p, _ := event["payload"].(map[string]any)
	return p
}
