package evo

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/project/wire"
)

func withLookupEnv(t *testing.T, env map[string]string) {
	t.Helper()
	t.Cleanup(swapLookupEnv(func(key string) string {
		if env == nil {
			return ""
		}
		return env[key]
	}))
}

func markTTY(t *testing.T, w io.Writer) {
	t.Helper()
	t.Cleanup(markWriterAsCharDevice(w))
}

func hasHumanGlyphs(s string) bool {
	return strings.ContainsAny(s, "✓✗⚠⊘") || strings.Contains(s, "\x1b[")
}

func hasLiveRegion(s string) bool {
	return strings.Contains(s, "\x1b[?25") || strings.Contains(s, "\x1b[2K")
}

func isolatedInit(t *testing.T, cfg Config) *Run {
	t.Helper()
	cfg.isolated = true
	out := initRun(cfg)
	t.Cleanup(func() { _ = out.close() })
	return out
}

func TestEVOOutput_Plain_NoLiveRegionOnTTYShapedWriter(t *testing.T) {
	withLookupEnv(t, map[string]string{"EVO_OUTPUT": "plain"})
	var buf bytes.Buffer
	markTTY(t, &buf)
	out := isolatedInit(t, Config{
		stdout:          &buf,
		stderr:          &buf,
		visibilityDelay: delay(0),
	})
	succeed(out.Task("scan").Doing("walk"), "ok")
	_ = out.finish()
	if hasLiveRegion(buf.String()) {
		t.Fatalf("EVO_OUTPUT=plain must not open a live region on a TTY-shaped writer:\n%q", buf.String())
	}
}

// EVO_OUTPUT=json selects the run document (FormatJSON) when the
// caller chose no Format: the one machine document that carries every
// structured Fact, disposition and Problem (E-090).
func TestEVOOutput_JSON_FinishWritesRunDocument(t *testing.T) {
	withLookupEnv(t, map[string]string{"EVO_OUTPUT": "json"})
	var buf bytes.Buffer
	out := isolatedInit(t, Config{stdout: &buf, stderr: io.Discard})
	succeed(out.Task("scan"), "ok")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	var doc struct {
		Object        string `json:"object"`
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("Finish must write one run document: %v\n%s", err, got)
	}
	if doc.Object != "run" || doc.SchemaVersion == "" {
		t.Fatalf("EVO_OUTPUT=json wrote %q schema %q, want an run document", doc.Object, doc.SchemaVersion)
	}
	if hasHumanGlyphs(got) {
		t.Fatalf("json stdout must not carry human glyphs:\n%s", got)
	}
}

// EVO_OUTPUT=jsonl selects the event stream (FormatJSONL).
func TestEVOOutput_JSONL_WritesEventLines(t *testing.T) {
	withLookupEnv(t, map[string]string{"EVO_OUTPUT": "jsonl"})
	var buf bytes.Buffer
	out := isolatedInit(t, Config{stdout: &buf, stderr: io.Discard})
	succeed(out.Task("scan"), "ok")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(buf.String())
	if got == "" {
		t.Fatal("jsonl wrote nothing")
	}
	var sawTaskDone bool
	for line := range strings.SplitSeq(got, "\n") {
		var ev struct {
			Object  string         `json:"object"`
			Type    string         `json:"type"`
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil || ev.Object != "event" {
			t.Fatalf("jsonl line is not an event (%v):\n%s", err, line)
		}
		if ev.Type == "task.finished" && ev.Payload["state"] == "done" {
			sawTaskDone = true
		}
		if hasHumanGlyphs(line) {
			t.Fatalf("jsonl line carries human glyphs: %s", line)
		}
	}
	if !sawTaskDone {
		t.Fatalf("jsonl missing a task.finished done event:\n%s", got)
	}
}

func TestEVOOutput_StreamJSON_TaskDoneEmitsEventJSONBeforeFinish(t *testing.T) {
	withLookupEnv(t, map[string]string{"EVO_OUTPUT": "stream-json"})
	var buf bytes.Buffer
	out := isolatedInit(t, Config{stdout: &buf, stderr: io.Discard})
	succeed(out.Task("scan"), "ok")
	got := strings.TrimSpace(buf.String())
	if got == "" {
		t.Fatal("stream-json wrote nothing at Task.Done")
	}
	var sawTaskDone bool
	for line := range strings.SplitSeq(got, "\n") {
		var ev wire.EventJSON
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("stream-json line before Finish is not EventJSON: %v\n%s", err, line)
		}
		if ev.Type == "task.done" {
			sawTaskDone = true
		}
	}
	if !sawTaskDone {
		t.Fatalf("Task.Done must emit a task.done EventJSON line before Finish:\n%s", got)
	}
	_ = out.finish()
}

func TestEVOOutput_StreamJSON_FormatDataKeepsPayloadOnStdout(t *testing.T) {
	withLookupEnv(t, map[string]string{"EVO_OUTPUT": "stream-json"})
	var stdout, stderr bytes.Buffer
	out := isolatedInit(t, Config{
		format: formatData,
		stdout: &stdout,
		stderr: &stderr,
	})
	succeed(out.Task("scan"), "ok")
	const payload = `{"ready":true}`
	if _, err := io.WriteString(out.ResultWriter(), payload); err != nil {
		t.Fatal(err)
	}
	beforeFinish := stderr.String()
	if strings.TrimSpace(beforeFinish) == "" {
		t.Fatal("FormatData+stream-json must write EventJSON to stderr at Task.Done")
	}
	var sawTaskDone bool
	for line := range strings.SplitSeq(strings.TrimSpace(beforeFinish), "\n") {
		var ev wire.EventJSON
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("stderr JSONL is not EventJSON: %v\n%s", err, line)
		}
		if ev.Type == "task.done" {
			sawTaskDone = true
		}
	}
	if !sawTaskDone {
		t.Fatalf("stderr missing task.done EventJSON:\n%s", beforeFinish)
	}
	if strings.Contains(stdout.String(), `"type"`) {
		t.Fatalf("presentation JSONL leaked onto stdout:\n%s", stdout.String())
	}
	if stdout.String() != payload {
		t.Fatalf("domain payload on stdout = %q, want %q", stdout.String(), payload)
	}
	_ = out.finish()
	if strings.Contains(stdout.String(), "scan") && !strings.Contains(stdout.String(), payload) {
		t.Fatalf("human presentation leaked onto FormatData stdout:\n%s", stdout.String())
	}
}

func TestEVOOutput_HumanDoesNotOverrideExplicitPlain(t *testing.T) {
	withLookupEnv(t, map[string]string{"EVO_OUTPUT": "human"})
	var buf bytes.Buffer
	markTTY(t, &buf)
	out := isolatedInit(t, Config{
		plain:           true,
		stdout:          &buf,
		stderr:          &buf,
		visibilityDelay: delay(0),
	})
	succeed(out.Task("scan").Doing("walk"), "ok")
	_ = out.finish()
	if hasLiveRegion(buf.String()) {
		t.Fatalf("Config.Plain: true must not be overridden by EVO_OUTPUT=human:\n%q", buf.String())
	}
}

func TestEVOOutput_NoColorStillDisablesColor(t *testing.T) {
	withLookupEnv(t, map[string]string{"NO_COLOR": "1"})
	var buf bytes.Buffer
	markTTY(t, &buf)
	out := isolatedInit(t, Config{
		plain:  true,
		stdout: &buf,
		stderr: &buf,
	})
	succeed(out.Task("ok"))
	out.Task("bad").fail("x")
	_ = out.finish()
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatalf("NO_COLOR must still disable color on a TTY-shaped writer:\n%q", buf.String())
	}
}

func TestEVOOutput_StreamJSONAliasUnderscore(t *testing.T) {
	withLookupEnv(t, map[string]string{"EVO_OUTPUT": "stream_json"})
	var buf bytes.Buffer
	out := isolatedInit(t, Config{stdout: &buf, stderr: io.Discard})
	succeed(out.Task("scan"))
	got := strings.TrimSpace(buf.String())
	if got == "" {
		t.Fatal("EVO_OUTPUT=stream_json alias wrote nothing at Task.Done")
	}
	var ev wire.EventJSON
	first, _, _ := strings.Cut(got, "\n")
	if err := json.Unmarshal([]byte(first), &ev); err != nil {
		t.Fatalf("stream_json alias must emit EventJSON: %v\n%s", err, first)
	}
	_ = out.finish()
}

func TestEVOVerbose_OneProjectsVerboseMessages(t *testing.T) {
	withLookupEnv(t, map[string]string{"EVO_VERBOSE": "1"})
	var buf bytes.Buffer
	out := isolatedInit(t, Config{stdout: &buf, stderr: &buf})
	out.atForTest(visibilityVerbose).println("hidden detail")
	_ = out.finish()
	if !strings.Contains(buf.String(), "hidden detail") {
		t.Fatalf("EVO_VERBOSE=1 must project verbose messages:\n%s", buf.String())
	}
}

func TestEVODebug_DebugLevelSurfacesJournal(t *testing.T) {
	withLookupEnv(t, map[string]string{"EVO_DEBUG": "debug"})
	var buf bytes.Buffer
	out := isolatedInit(t, Config{stdout: &buf, stderr: &buf})
	out.debugForTest("trace-visible")
	succeed(out.Task("ok"))
	_ = out.finish()
	if !strings.Contains(buf.String(), "trace-visible") {
		t.Fatalf("EVO_DEBUG=debug must surface Debug journal:\n%s", buf.String())
	}
}
