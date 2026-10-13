package evo

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

// TestSummary_ReachesRunDocumentAndEventStream pins E-115 (ZYS-971): a
// Task's Summary is a structured `summary` field in machine output. The
// run task entry and the task.finished event carried none, so a JSON
// or JSONL consumer lost every Task Summary.
func TestSummary_ReachesRunDocumentAndEventStream(t *testing.T) {
	var events bytes.Buffer
	out := initRun(Config{isolated: true, stateDir: t.TempDir(), stdout: &events, title: "sum", format: formatJSONL})
	task := out.Task("check branches")
	task.Define(func(context.Context) error { task.Summary("459 checked"); return nil })
	_ = out.finish()
	var js bytes.Buffer
	if err := writeJSON(&js, Result{Conclusion: out.conclusion()}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	_ = out.close()

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
		t.Errorf("run tasks = %+v, want one with summary %q\n%s", doc.Data.Tasks, "459 checked", js.String())
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
