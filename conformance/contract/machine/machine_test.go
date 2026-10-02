package machine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/conformance/contract/harness"
)

// wire builds an isolated Output whose two streams are captured separately.
func wire(t *testing.T, mutate func(*evo.Config)) (*evo.Output, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cfg := evo.Config{Isolated: true, Title: "run", Stdout: &stdout, Stderr: &stderr}
	mutate(&cfg)
	out := evo.Init(cfg)
	t.Cleanup(func() { _ = out.Close() })
	return out, &stdout, &stderr
}

func failingRun(out *evo.Output) {
	_ = out.Task("ok").Define(func(context.Context) error { return nil }).Wait()
	_ = out.Task("bad").Define(func(context.Context) error { return errors.New("boom") }).Wait()
	_ = out.Finish()
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
}

func TestC16_001_FormatJSONWritesExactlyOneRunDocumentToStdout(t *testing.T) {
	unsetEnv(t, "EVO_OUTPUT")
	out, stdout, stderr := wire(t, func(c *evo.Config) { c.Format = evo.FormatJSON })
	failingRun(out)
	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil || dec.More() {
		t.Fatalf("stdout is not exactly one JSON document (err=%v):\n%s", err, stdout)
	}
	if doc["object"] != "evo.run" || doc["schema_version"] != "2.0" {
		t.Fatalf("document = %v %v", doc["object"], doc["schema_version"])
	}
	if !strings.Contains(stderr.String(), "boom") {
		t.Fatalf("human presentation missing from stderr:\n%s", stderr)
	}
}

func TestC16_002_FormatJSONLStreamsMonotonicEventsEndingWithRunFinished(t *testing.T) {
	unsetEnv(t, "EVO_OUTPUT")
	out, stdout, _ := wire(t, func(c *evo.Config) { c.Format = evo.FormatJSONL })
	failingRun(out)
	events := harness.JSONLines(t, stdout.String())
	var last float64
	for _, e := range events {
		if e["schema_version"] != "1.0" {
			t.Fatalf("event schema_version = %v", e["schema_version"])
		}
		seq, _ := e["seq"].(float64)
		if seq <= last {
			t.Fatalf("seq %v not strictly after %v", seq, last)
		}
		last = seq
	}
	if got := events[len(events)-1]["type"]; got != "run.finished" {
		t.Fatalf("last event = %v, want run.finished", got)
	}
}

func TestC16_003_ExplicitConfigWinsOverEnvironmentWhichWinsOverInference(t *testing.T) {
	t.Setenv("EVO_OUTPUT", "json")
	out, stdout, _ := wire(t, func(*evo.Config) {})
	failingRun(out)
	var doc map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil || doc["object"] != "evo.run" {
		t.Fatalf("EVO_OUTPUT=json did not select FormatJSON (err=%v): %s", err, stdout)
	}
	out2, stdout2, _ := wire(t, func(c *evo.Config) { c.Format = evo.FormatJSONL })
	failingRun(out2)
	if lines := harness.JSONLines(t, stdout2.String()); len(lines) < 2 || lines[0]["object"] == "evo.run" {
		t.Fatalf("explicit FormatJSONL lost to the environment: %s", stdout2)
	}
}

func TestC16_004_PipedStdoutNeverInfersMachineOutput(t *testing.T) {
	unsetEnv(t, "EVO_OUTPUT")
	out, stdout, _ := wire(t, func(*evo.Config) {})
	failingRun(out)
	if strings.HasPrefix(strings.TrimSpace(stdout.String()), "{") {
		t.Fatalf("stdout became machine output without being asked:\n%s", stdout)
	}
}

func TestC16_005_TaskEntryCarriesEveryContractField(t *testing.T) {
	out, _ := harness.New(t)
	g := out.Group("g")
	task := g.Task("t")
	task.Summary("done it")
	task.Define(func(context.Context) error {
		task.Fact("k", "v")
		task.Problem("drift", evo.Severity(evo.SeverityWarning))
		return nil
	})
	_ = g.Wait()
	doc := harness.RunDocument(t, out)
	var entry map[string]any
	for _, candidate := range harness.Objects(t, harness.Object(t, doc, "data"), "tasks") {
		if candidate["name"] == "t" {
			entry = candidate
		}
	}
	for _, key := range []string{
		"id", "key", "parent_id", "name", "state", "resolution", "definition_executed", "evidence",
		"progress", "activity", "timing", "verification", "tracked_resources", "basis", "facts",
		"dispositions", "problems", "warnings", "operations", "summary",
	} {
		if _, ok := entry[key]; !ok {
			t.Errorf("task entry lacks %q: %v", key, entry)
		}
	}
}

func TestC16_006_CollectionsAndTopLevelDataCarryTheContractedKeys(t *testing.T) {
	out, _ := harness.New(t)
	g := out.Group("g")
	g.Summary("group summary")
	t1 := g.Task("t")
	t1.Define(func(ctx context.Context) error {
		return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectCreate, Object: "file", Quantity: 1},
			func(context.Context) error { return nil })
	})
	_ = g.Wait()
	out.Fact("root", "/r")
	out.NextCommand("zq", "retry")
	_ = out.Task("bad").Define(func(context.Context) error { return errors.New("boom") }).Wait()
	doc := harness.RunDocument(t, out)
	data := harness.Object(t, doc, "data")
	for _, key := range []string{"tasks", "collections", "effects", "facts", "problems", "actions"} {
		if _, ok := data[key]; !ok {
			t.Errorf("data lacks %q: %v", key, data)
		}
	}
	collection := harness.Objects(t, data, "collections")[0]
	for _, key := range []string{"kind", "state", "summary", "children"} {
		if _, ok := collection[key]; !ok {
			t.Errorf("collection lacks %q: %v", key, collection)
		}
	}
}

func TestC16_007_RunDocumentPreservesIdentityTimingModeAndOutcome(t *testing.T) {
	out, _ := harness.New(t)
	failingRun(out)
	doc := harness.RunDocument(t, out)
	for _, key := range []string{"run_id", "started_at", "finished_at", "duration_ms", "mode", "outcome", "exit_code"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("document lacks %q", key)
		}
	}
	if doc["mode"] != "apply" || doc["outcome"] != "failed" || fmt.Sprint(doc["exit_code"]) != "2" {
		t.Fatalf("mode=%v outcome=%v exit=%v", doc["mode"], doc["outcome"], doc["exit_code"])
	}
}

func TestC16_008_EffectsAreTaggedPlannedOrChanged(t *testing.T) {
	for _, dry := range []bool{true, false} {
		out, _ := harness.New(t, func(c *evo.Config) { c.DryRun = dry })
		_ = out.Task("prune").Define(func(ctx context.Context) error {
			return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectDelete, Object: "tip", Quantity: 2},
				func(context.Context) error { return nil })
		}).Wait()
		effects := harness.Objects(t, harness.Object(t, harness.RunDocument(t, out), "data"), "effects")
		want := "changed"
		if dry {
			want = "planned"
		}
		if len(effects) != 1 || effects[0]["status"] != want {
			t.Fatalf("dry=%v effects = %v", dry, effects)
		}
	}
}

func TestC16_009_PlainJSONAndJSONLAgreeOnTheRunOutcome(t *testing.T) {
	unsetEnv(t, "EVO_OUTPUT")
	plain, buf := harness.New(t)
	failingRun(plain)
	jsonOut, stdout, _ := wire(t, func(c *evo.Config) { c.Format = evo.FormatJSON })
	failingRun(jsonOut)
	jsonlOut, jsonl, _ := wire(t, func(c *evo.Config) { c.Format = evo.FormatJSONL })
	failingRun(jsonlOut)
	var doc struct {
		Outcome string `json:"outcome"`
		Data    struct {
			Tasks []struct{ Name, State string }
		}
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, task := range doc.Data.Tasks {
		states[task.Name] = task.State
	}
	if doc.Outcome != "failed" || states["ok"] != "done" || states["bad"] != "failed" {
		t.Fatalf("json: %s %v", doc.Outcome, states)
	}
	if !strings.Contains(buf.String(), "✓ ok") || !strings.Contains(buf.String(), "✗ bad") || !strings.Contains(buf.String(), "[failed") {
		t.Fatalf("plain:\n%s", buf)
	}
	if events := harness.JSONLines(t, jsonl.String()); !strings.Contains(jsonl.String(), `"failed"`) || len(events) < 3 {
		t.Fatalf("jsonl lacks the failed state:\n%s", jsonl)
	}
}
