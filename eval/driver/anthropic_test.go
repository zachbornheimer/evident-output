package driver_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/zachbornheimer/evident-output/eval/driver"
)

const fakeCredential = "test-credential-not-real"

const okReply = `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5-5",
"content":[{"type":"text","text":"hi"},{"type":"tool_use","id":"tu_1","name":"go_build","input":{"files":{"main.go":"package main"}}}],
"stop_reason":"tool_use","stop_sequence":null,
"usage":{"input_tokens":11,"output_tokens":22,"cache_read_input_tokens":33,"cache_creation_input_tokens":44}}`

func sampleRequest() driver.Request {
	schema := json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`)
	return driver.Request{
		Model: driver.ModelInnerLoop, MaxTokens: 100, System: "the docs",
		Tools: []driver.ToolDef{
			{Name: "first", Description: "d1", InputSchema: schema},
			{Name: "last", Description: "d2", InputSchema: schema},
		},
		Messages: []driver.Message{{Role: driver.RoleUser, Blocks: []driver.Block{{Kind: driver.BlockText, Text: "go"}}}},
	}
}

func marshalParams(t *testing.T, req driver.Request) map[string]any {
	t.Helper()
	params, err := driver.BuildParams(req)
	if err != nil {
		t.Fatalf("BuildParams: %v", err)
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	return wire
}

func hasCacheControl(block any) bool {
	entry, _ := block.(map[string]any)
	control, ok := entry["cache_control"].(map[string]any)
	return ok && control["type"] == "ephemeral"
}

// The cache breakpoint sits on the last system block and the last tool, and
// nowhere else.
func TestBuildParams_CacheControlOnLastSystemBlockAndLastTool(t *testing.T) {
	wire := marshalParams(t, sampleRequest())
	system, _ := wire["system"].([]any)
	if len(system) != 1 || !hasCacheControl(system[0]) {
		t.Fatalf("system blocks must end in a cache breakpoint: %v", wire["system"])
	}
	tools, _ := wire["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("want 2 tools, got %v", wire["tools"])
	}
	if hasCacheControl(tools[0]) {
		t.Error("first tool must not carry a cache breakpoint")
	}
	if !hasCacheControl(tools[1]) {
		t.Error("last tool must carry the cache breakpoint")
	}
}

func TestBuildParams_UsesOnlyStandardParameters(t *testing.T) {
	wire := marshalParams(t, sampleRequest())
	allowed := map[string]bool{"model": true, "max_tokens": true, "system": true, "tools": true, "messages": true}
	for key := range wire {
		if !allowed[key] {
			t.Errorf("unexpected request parameter %q", key)
		}
	}
}

func replyServer(t *testing.T, replies []func(http.ResponseWriter)) (*httptest.Server, *atomic.Int32, *[]byte) {
	t.Helper()
	var calls atomic.Int32
	var lastBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		lastBody = body
		index := int(calls.Add(1)) - 1
		w.Header().Set("Content-Type", "application/json")
		replies[min(index, len(replies)-1)](w)
	}))
	t.Cleanup(server.Close)
	return server, &calls, &lastBody
}

func TestAnthropicModel_MapsResponseAndUsage(t *testing.T) {
	server, _, body := replyServer(t, []func(http.ResponseWriter){func(w http.ResponseWriter) { _, _ = io.WriteString(w, okReply) }})
	model := driver.NewAnthropicModel(fakeCredential, option.WithBaseURL(server.URL))
	resp, usage, err := model.Complete(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	want := driver.Usage{InputTokens: 11, OutputTokens: 22, CacheReadTokens: 33, CacheWriteTokens: 44}
	if usage != want {
		t.Errorf("usage = %+v, want %+v", usage, want)
	}
	uses := resp.ToolUses()
	if resp.Stop != driver.StopToolUse || len(uses) != 1 || uses[0].ToolName != "go_build" || uses[0].ToolUseID != "tu_1" {
		t.Errorf("response mapped wrong: %+v", resp)
	}
	var sent map[string]any
	if err := json.Unmarshal(*body, &sent); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	system, _ := sent["system"].([]any)
	if len(system) != 1 || !hasCacheControl(system[0]) {
		t.Errorf("wire request lost the system cache breakpoint: %v", sent["system"])
	}
}

func TestAnthropicModel_RetriesRateLimitThenSucceeds(t *testing.T) {
	limited := func(w http.ResponseWriter) {
		w.Header().Set("retry-after-ms", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)
	}
	ok := func(w http.ResponseWriter) { _, _ = io.WriteString(w, okReply) }
	server, calls, _ := replyServer(t, []func(http.ResponseWriter){limited, ok})
	model := driver.NewAnthropicModel(fakeCredential, option.WithBaseURL(server.URL))
	if _, _, err := model.Complete(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("Complete after retry: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("want 2 requests (one retry), got %d", got)
	}
}

func TestAnthropicModel_GivesUpAfterBoundedRetries(t *testing.T) {
	failing := func(w http.ResponseWriter) {
		w.Header().Set("retry-after-ms", "1")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"api_error","message":"down"}}`)
	}
	server, calls, _ := replyServer(t, []func(http.ResponseWriter){failing})
	model := driver.NewAnthropicModel(fakeCredential, option.WithBaseURL(server.URL))
	if _, _, err := model.Complete(context.Background(), sampleRequest()); err == nil {
		t.Fatal("want an error after exhausting retries")
	}
	if got := calls.Load(); got != driver.DefaultMaxRetries+1 {
		t.Errorf("want %d requests, got %d", driver.DefaultMaxRetries+1, got)
	}
}
