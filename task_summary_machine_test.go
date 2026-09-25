package evo_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestSummary_ReachesRunDocumentAndEventStream pins E-115 (ZYS-971): a
// Task's Summary is a structured `summary` field in machine output. The
// evo.run task entry and the task.finished event carried none, so a JSON
// or JSONL consumer lost every Task Summary.
func TestSummary_ReachesRunDocumentAndEventStream(t *testing.T) {
	var events bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, StateDir: t.TempDir(), Stdout: &events, Title: "sum", Format: evo.FormatJSONL})
	task := out.Task("check branches")
	task.Define(func(context.Context) error { task.Summary("459 checked"); return nil })
	_ = out.Finish()
	var js bytes.Buffer
	if err := evo.WriteJSON(&js, evo.Result{Conclusion: out.Conclusion()}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	_ = out.Close()

	var doc struct {
		Data struct {
			Tasks []struct {
				Name    string `json:"name"`
				Summary string `json:"summary"`
			} `json:"tasks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(js.Bytes(), &doc); err != nil {
		t.Fatalf("decode run document: %v\n%s", err, js.String())
	}
	if len(doc.Data.Tasks) != 1 || doc.Data.Tasks[0].Summary != "459 checked" {
		t.Errorf("evo.run tasks = %+v, want one with summary %q\n%s", doc.Data.Tasks, "459 checked", js.String())
	}

	found := false
	for sc := bufio.NewScanner(&events); sc.Scan(); {
		var ev struct {
			Type    string         `json:"type"`
			Payload map[string]any `json:"payload"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.Type != "task.finished" {
			continue
		}
		found = true
		if ev.Payload["summary"] != "459 checked" {
			t.Errorf("task.finished payload = %v, want summary %q", ev.Payload, "459 checked")
		}
	}
	if !found {
		t.Errorf("no task.finished event in:\n%s", events.String())
	}
}
