package driver_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/eval/driver"
	"github.com/zachbornheimer/evident-output/internal/agent/evaltask"
)

var goodFiles = map[string]any{"files": map[string]string{"main.go": "package main\n"}}

// newSample builds a Sample whose go_build is never called by these loop tests.
func newSample(model driver.Model, server driver.ToolServer, guard *driver.SpendGuard) driver.Sample {
	return driver.Sample{
		Model: model, ModelID: driver.ModelInnerLoop, MaxTokens: 10, System: "docs", Prompt: "task",
		Tools: driver.Toolbox{Server: server, Builder: driver.Builder{Task: evaltask.Task{ID: "t"}}},
		Meter: driver.Meter{Price: driver.Price{InputPerMTok: 1_000_000, Verified: true}, Guard: guard},
	}
}

func bigGuard(t *testing.T) *driver.SpendGuard {
	t.Helper()
	guard, err := driver.NewSpendGuard(1e12)
	if err != nil {
		t.Fatalf("NewSpendGuard: %v", err)
	}
	return guard
}

func TestSample_SubmitEndsTheSampleAndAccountsUsage(t *testing.T) {
	usage := driver.Usage{InputTokens: 2, OutputTokens: 3}
	model := &driver.ScriptedModel{Turns: []driver.ScriptedTurn{
		driver.ToolCallTurn("1", "evident_output_explain", map[string]any{"rule_id": "API-006"}, usage),
		driver.ToolCallTurn("2", "submit", goodFiles, usage),
	}}
	server := &fakeServer{replies: map[string]driver.ToolResult{"evident_output_explain": {Text: "rule"}}}
	result, err := newSample(model, server, bigGuard(t)).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Submitted || result.Turns != 2 || result.ToolCalls != 2 || result.Files["main.go"] == "" {
		t.Errorf("result = %+v", result)
	}
	if want := (driver.Usage{InputTokens: 4, OutputTokens: 6}); result.Usage != want {
		t.Errorf("usage = %+v, want %+v", result.Usage, want)
	}
	if result.CostUSD != 4 {
		t.Errorf("cost = %v, want 4 (4 input tokens at $1 each)", result.CostUSD)
	}
}

func TestSample_ModelSeesDocsAndToolsButNoHostTools(t *testing.T) {
	model := &driver.ScriptedModel{Turns: []driver.ScriptedTurn{driver.ToolCallTurn("1", "submit", goodFiles, driver.Usage{})}}
	if _, err := newSample(model, &fakeServer{}, bigGuard(t)).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	req := model.Requests[0]
	if req.System != "docs" {
		t.Errorf("system prompt = %q", req.System)
	}
	var names []string
	for _, tool := range req.Tools {
		names = append(names, tool.Name)
	}
	for _, banned := range []string{"evident_output_adopt_plan", "evident_output_update"} {
		if strings.Contains(strings.Join(names, ","), banned) {
			t.Errorf("tool %s must not be offered", banned)
		}
	}
	if names[len(names)-1] != "submit" || !strings.Contains(strings.Join(names, ","), "go_build") {
		t.Errorf("tools = %v: want go_build offered and submit last", names)
	}
}

func TestSample_TurnCapEndsWithoutSubmission(t *testing.T) {
	var turns []driver.ScriptedTurn
	for range driver.MaxToolTurns + 5 {
		turns = append(turns, driver.ToolCallTurn("x", "evident_output_explain", map[string]any{}, driver.Usage{}))
	}
	server := &fakeServer{replies: map[string]driver.ToolResult{"evident_output_explain": {Text: "r"}}}
	result, err := newSample(&driver.ScriptedModel{Turns: turns}, server, bigGuard(t)).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Submitted || result.Turns != driver.MaxToolTurns || result.Ended != driver.EndedTurnCap {
		t.Errorf("result = %+v", result)
	}
}

func TestSample_TextOnlyReplyEndsWithoutSubmission(t *testing.T) {
	model := &driver.ScriptedModel{Turns: []driver.ScriptedTurn{driver.TextTurn("I give up", driver.Usage{})}}
	result, err := newSample(model, &fakeServer{}, bigGuard(t)).Run(context.Background())
	if err != nil || result.Submitted || result.Ended != driver.EndedNoSubmit {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
}

func TestSample_CyclesToCleanCountsReviewCallsUntilFirstClean(t *testing.T) {
	review := func(id string) driver.ScriptedTurn {
		return driver.ToolCallTurn(id, "evident_output_review", map[string]any{"source": "package main"}, driver.Usage{})
	}
	model := &driver.ScriptedModel{Turns: []driver.ScriptedTurn{review("1"), review("2"), review("3"), driver.ToolCallTurn("4", "submit", goodFiles, driver.Usage{})}}
	dirty := driver.ToolResult{Structured: json.RawMessage(`{"recheck_required":true}`)}
	clean := driver.ToolResult{Structured: json.RawMessage(`{"recheck_required":false}`)}
	server := &sequenceServer{results: []driver.ToolResult{dirty, clean, clean}}
	result, err := newSample(model, server, bigGuard(t)).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.CyclesToClean != 2 {
		t.Errorf("cycles to clean = %d, want 2", result.CyclesToClean)
	}
}

func TestSample_SpendCapStopsBeforeNextRequest(t *testing.T) {
	guard, err := driver.NewSpendGuard(5)
	if err != nil {
		t.Fatalf("NewSpendGuard: %v", err)
	}
	expensive := driver.Usage{InputTokens: 3}
	var turns []driver.ScriptedTurn
	for range 5 {
		turns = append(turns, driver.ToolCallTurn("x", "evident_output_explain", map[string]any{}, expensive))
	}
	model := &driver.ScriptedModel{Turns: turns}
	server := &fakeServer{replies: map[string]driver.ToolResult{"evident_output_explain": {Text: "r"}}}
	result, err := newSample(model, server, guard).Run(context.Background())
	if !errors.Is(err, driver.ErrSpendCapReached) {
		t.Fatalf("err = %v, want ErrSpendCapReached", err)
	}
	if len(model.Requests) != 2 || result.Submitted {
		t.Errorf("want the run to stop after the 2nd request ($6 >= $5); requests = %d", len(model.Requests))
	}
}

func TestToolbox_BlocksHostPathArguments(t *testing.T) {
	box := driver.Toolbox{Server: &fakeServer{}}
	for _, argument := range []string{"directory", "file"} {
		input, _ := json.Marshal(map[string]string{argument: "/etc"})
		outcome, err := box.Dispatch(context.Background(), driver.Block{ToolName: "evident_output_review", Input: input})
		if err != nil || !outcome.IsError {
			t.Errorf("%s argument must be refused: %+v %v", argument, outcome, err)
		}
	}
}

func TestToolbox_UnknownAndUnforwardedToolsAreErrors(t *testing.T) {
	server := &fakeServer{}
	box := driver.Toolbox{Server: server}
	for _, name := range []string{"bash", "evident_output_update", "evident_output_adopt_plan"} {
		outcome, err := box.Dispatch(context.Background(), driver.Block{ToolName: name, Input: json.RawMessage(`{}`)})
		if err != nil || !outcome.IsError {
			t.Errorf("%s must be refused: %+v %v", name, outcome, err)
		}
	}
	if len(server.calls) != 0 {
		t.Errorf("refused tools reached the MCP server: %v", server.calls)
	}
}

func TestValidateFiles(t *testing.T) {
	if err := driver.ValidateFiles(map[string]string{"main.go": "package main"}); err != nil {
		t.Errorf("valid files rejected: %v", err)
	}
	for _, bad := range []string{"../main.go", "dir/main.go", "main.txt", "main_test.go", ""} {
		if err := driver.ValidateFiles(map[string]string{bad: "x"}); err == nil {
			t.Errorf("file name %q must be rejected", bad)
		}
	}
	if err := driver.ValidateFiles(nil); err == nil {
		t.Error("no files must be rejected")
	}
}

// sequenceServer answers every call with the next canned result.
type sequenceServer struct {
	fakeServer
	results []driver.ToolResult
	next    int
}

func (s *sequenceServer) Call(context.Context, string, json.RawMessage) (driver.ToolResult, error) {
	result := s.results[s.next]
	s.next++
	return result, nil
}
