package evo_test

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func withLookupEnv(t *testing.T, env map[string]string) {
	t.Helper()
	t.Cleanup(evo.SwapLookupEnv(func(key string) string {
		if env == nil {
			return ""
		}
		return env[key]
	}))
}

func markTTY(t *testing.T, w io.Writer) {
	t.Helper()
	t.Cleanup(evo.MarkWriterAsCharDevice(w))
}

func hasHumanGlyphs(s string) bool {
	return strings.ContainsAny(s, "✓✗⚠⊘") || strings.Contains(s, "\x1b[")
}

func hasLiveRegion(s string) bool {
	return strings.Contains(s, "\x1b[?25") || strings.Contains(s, "\x1b[2K")
}

func isolatedInit(t *testing.T, cfg evo.Config) *evo.Output {
	t.Helper()
	cfg.Isolated = true
	out := evo.Init(cfg)
	t.Cleanup(func() { _ = out.Close() })
	return out
}

func TestEVOOutput_Plain_NoLiveRegionOnTTYShapedWriter(t *testing.T) {
	withLookupEnv(t, map[string]string{"EVO_OUTPUT": "plain"})
	var buf bytes.Buffer
	markTTY(t, &buf)
	out := isolatedInit(t, evo.Config{
		Stdout:          &buf,
		Stderr:          &buf,
		VisibilityDelay: evo.Delay(0),
	})
	out.Task("scan").Doing("walk").Done("ok")
	_ = out.Finish()
	if hasLiveRegion(buf.String()) {
		t.Fatalf("EVO_OUTPUT=plain must not open a live region on a TTY-shaped writer:\n%q", buf.String())
	}
}

func TestEVOOutput_JSON_FinishWritesJSONDocument(t *testing.T) {
	withLookupEnv(t, map[string]string{"EVO_OUTPUT": "json"})
	var buf bytes.Buffer
	out := isolatedInit(t, evo.Config{Stdout: &buf, Stderr: io.Discard})
	out.Task("scan").Done("ok")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	var doc evo.JSONDocument
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("Finish must write a JSONDocument: %v\n%s", err, got)
	}
	if doc.SchemaVersion == "" {
		t.Fatalf("JSONDocument missing schema_version: %+v", doc)
	}
	if hasHumanGlyphs(got) {
		t.Fatalf("json presentation must not carry human glyphs:\n%s", got)
	}
}

func TestEVOOutput_JSONL_FinishWritesEventLines(t *testing.T) {
	withLookupEnv(t, map[string]string{"EVO_OUTPUT": "jsonl"})
	var buf bytes.Buffer
	out := isolatedInit(t, evo.Config{Stdout: &buf, Stderr: io.Discard})
	out.Task("scan").Done("ok")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(buf.String())
	if got == "" {
		t.Fatal("jsonl Finish wrote nothing")
	}
	var sawTaskDone bool
	for _, line := range strings.Split(got, "\n") {
		var ev evo.EventJSON
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("jsonl line is not EventJSON: %v\n%s", err, line)
		}
		if ev.Type == "task.done" {
			sawTaskDone = true
		}
		if hasHumanGlyphs(line) {
			t.Fatalf("jsonl line carries human glyphs: %s", line)
		}
	}
	if !sawTaskDone {
		t.Fatalf("jsonl Finish missing task.done event:\n%s", got)
	}
}

func TestEVOOutput_StreamJSON_TaskDoneEmitsEventJSONBeforeFinish(t *testing.T) {
	withLookupEnv(t, map[string]string{"EVO_OUTPUT": "stream-json"})
	var buf bytes.Buffer
	out := isolatedInit(t, evo.Config{Stdout: &buf, Stderr: io.Discard})
	out.Task("scan").Done("ok")
	got := strings.TrimSpace(buf.String())
	if got == "" {
		t.Fatal("stream-json wrote nothing at Task.Done")
	}
	var sawTaskDone bool
	for _, line := range strings.Split(got, "\n") {
		var ev evo.EventJSON
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
	_ = out.Finish()
}

func TestEVOOutput_StreamJSON_FormatDataKeepsPayloadOnStdout(t *testing.T) {
	withLookupEnv(t, map[string]string{"EVO_OUTPUT": "stream-json"})
	var stdout, stderr bytes.Buffer
	out := isolatedInit(t, evo.Config{
		Format: evo.FormatData,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	out.Task("scan").Done("ok")
	const payload = `{"ready":true}`
	if _, err := io.WriteString(out.ResultWriter(), payload); err != nil {
		t.Fatal(err)
	}
	beforeFinish := stderr.String()
	if strings.TrimSpace(beforeFinish) == "" {
		t.Fatal("FormatData+stream-json must write EventJSON to stderr at Task.Done")
	}
	var sawTaskDone bool
	for _, line := range strings.Split(strings.TrimSpace(beforeFinish), "\n") {
		var ev evo.EventJSON
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
	_ = out.Finish()
	if strings.Contains(stdout.String(), "scan") && !strings.Contains(stdout.String(), payload) {
		t.Fatalf("human presentation leaked onto FormatData stdout:\n%s", stdout.String())
	}
}

func TestEVOOutput_HumanDoesNotOverrideExplicitPlain(t *testing.T) {
	withLookupEnv(t, map[string]string{"EVO_OUTPUT": "human"})
	var buf bytes.Buffer
	markTTY(t, &buf)
	out := isolatedInit(t, evo.Config{
		Plain:           true,
		Stdout:          &buf,
		Stderr:          &buf,
		VisibilityDelay: evo.Delay(0),
	})
	out.Task("scan").Doing("walk").Done("ok")
	_ = out.Finish()
	if hasLiveRegion(buf.String()) {
		t.Fatalf("Config.Plain: true must not be overridden by EVO_OUTPUT=human:\n%q", buf.String())
	}
}

func TestEVOOutput_NoColorStillDisablesColor(t *testing.T) {
	withLookupEnv(t, map[string]string{"NO_COLOR": "1"})
	var buf bytes.Buffer
	markTTY(t, &buf)
	out := isolatedInit(t, evo.Config{
		Plain:  true,
		Stdout: &buf,
		Stderr: &buf,
	})
	out.Task("ok").Done()
	out.Task("bad").Fail("x")
	_ = out.Finish()
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatalf("NO_COLOR must still disable color on a TTY-shaped writer:\n%q", buf.String())
	}
}

func TestEVOOutput_StreamJSONAliasUnderscore(t *testing.T) {
	withLookupEnv(t, map[string]string{"EVO_OUTPUT": "stream_json"})
	var buf bytes.Buffer
	out := isolatedInit(t, evo.Config{Stdout: &buf, Stderr: io.Discard})
	out.Task("scan").Done()
	got := strings.TrimSpace(buf.String())
	if got == "" {
		t.Fatal("EVO_OUTPUT=stream_json alias wrote nothing at Task.Done")
	}
	var ev evo.EventJSON
	first := strings.SplitN(got, "\n", 2)[0]
	if err := json.Unmarshal([]byte(first), &ev); err != nil {
		t.Fatalf("stream_json alias must emit EventJSON: %v\n%s", err, first)
	}
	_ = out.Finish()
}

func TestEVOVerbose_OneProjectsVerboseMessages(t *testing.T) {
	withLookupEnv(t, map[string]string{"EVO_VERBOSE": "1"})
	var buf bytes.Buffer
	out := isolatedInit(t, evo.Config{Stdout: &buf, Stderr: &buf})
	out.At(evo.VisibilityVerbose).Println("hidden detail")
	_ = out.Finish()
	if !strings.Contains(buf.String(), "hidden detail") {
		t.Fatalf("EVO_VERBOSE=1 must project verbose messages:\n%s", buf.String())
	}
}

func TestEVODebug_DebugLevelSurfacesJournal(t *testing.T) {
	withLookupEnv(t, map[string]string{"EVO_DEBUG": "debug"})
	var buf bytes.Buffer
	out := isolatedInit(t, evo.Config{Stdout: &buf, Stderr: &buf})
	out.Debug("trace-visible")
	out.Task("ok").Done()
	_ = out.Finish()
	if !strings.Contains(buf.String(), "trace-visible") {
		t.Fatalf("EVO_DEBUG=debug must surface Debug journal:\n%s", buf.String())
	}
}
