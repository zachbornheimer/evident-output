package evo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestParseFormat(t *testing.T) {
	cases := []struct {
		in      string
		want    format
		wantErr bool
	}{
		{"human", formatHuman, false},
		{"  human  ", formatHuman, false},
		{"HUMAN", formatHuman, false},
		{"data", formatData, false},
		{"external", formatExternal, false},
		{"json", formatJSON, false},
		{"jsonl", formatJSONL, false},
		{"JSONL", formatJSONL, false},
		{"yaml", 0, true},
		{"", 0, true},
	}
	for _, tc := range cases {
		got, err := parseFormat(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseFormat(%q): want error, got nil", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseFormat(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseFormat(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// nopFlushWriter records writes without ever failing — a stand-in for a
// real stdout/stderr pipe in these Format-routing tests.
type nopFlushWriter struct{ bytes.Buffer }

func (w *nopFlushWriter) Flush() error { return nil }

// erroringWriter always fails — the write-failure path spec §32.2 requires
// to escalate an otherwise-OK exit code to ExitFailed (2).
type erroringWriter struct{}

func (erroringWriter) Write(p []byte) (int, error) {
	return 0, errors.New("erroringWriter: write failed")
}

func TestFormatJSON_StdoutHasOneRunDocumentStderrHasHuman(t *testing.T) {
	var stdout, stderr nopFlushWriter
	out := initRun(Config{
		isolated: true, title: "demo", format: formatJSON,
		stdout: &stdout, stderr: &stderr,
	})
	out.Task("build").succeed("")
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	stdoutBody := stdout.String()
	if strings.Count(stdoutBody, `"object": "evo.run"`) != 1 {
		t.Fatalf("stdout must contain exactly one evo.run document, got:\n%s", stdoutBody)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdoutBody), &doc); err != nil {
		t.Fatalf("stdout is not one valid JSON document: %v\nstdout:\n%s", err, stdoutBody)
	}
	if !strings.Contains(stdoutBody, `"name": "build"`) {
		t.Fatalf("evo.run document must carry the declared task, got:\n%s", stdoutBody)
	}
	if strings.Contains(stdoutBody, "evo.event") {
		t.Fatalf("FormatJSON must never write evo.event lines to stdout, got:\n%s", stdoutBody)
	}

	stderrBody := stderr.String()
	if stderrBody == "" {
		t.Fatalf("human presentation must still stream to stderr under FormatJSON")
	}
	if strings.Contains(stderrBody, `"object": "evo.run"`) {
		t.Fatalf("the evo.run document must never appear on stderr, got:\n%s", stderrBody)
	}
}

func TestFormatJSON_StdoutWriteFailureEscalatesToExitFailed(t *testing.T) {
	var stderr nopFlushWriter
	out := initRun(Config{
		isolated: true, title: "demo", format: formatJSON,
		stdout: erroringWriter{}, stderr: &stderr,
	})
	result := out.run(context.Background(), func(context.Context) error {
		out.Task("build").succeed("")
		return nil
	})
	if result.ExitCode() != exitFailed {
		t.Fatalf("Result.ExitCode() = %d, want ExitFailed (%d) after a stdout write failure", result.ExitCode(), exitFailed)
	}
}

func TestFormatJSONL_StdoutStreamsEventLinesStderrHasHuman(t *testing.T) {
	var stdout, stderr nopFlushWriter
	out := initRun(Config{
		isolated: true, title: "demo", format: formatJSONL,
		stdout: &stdout, stderr: &stderr,
	})
	out.Task("build").succeed("")
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected multiple evo.event JSONL lines, got %d:\n%s", len(lines), stdout.String())
	}
	var lastSeq uint64
	for i, line := range lines {
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("line %d is not valid JSON: %v\nline: %s", i, err, line)
		}
		if e["object"] != "evo.event" {
			t.Fatalf("line %d object = %v, want evo.event", i, e["object"])
		}
		seq := uint64(e["seq"].(float64))
		if seq <= lastSeq {
			t.Fatalf("line %d seq %d is not strictly increasing after %d", i, seq, lastSeq)
		}
		lastSeq = seq
	}
	if strings.Contains(stdout.String(), `"object":"evo.run"`) {
		t.Fatalf("FormatJSONL must never write an evo.run document to stdout, got:\n%s", stdout.String())
	}

	if stderr.String() == "" {
		t.Fatalf("human presentation must still stream to stderr under FormatJSONL")
	}
}

// hasLiveRegionExt reports whether s carries the ANSI live-region markers
// terminal.ANSI.WriteLive emits (cursor-hide / erase-line) — the same test
// used by the root package's env_output_test.go to detect an armed live
// surface without depending on a real pty.
func hasLiveRegionExt(s string) bool {
	return strings.Contains(s, "\x1b[?25") || strings.Contains(s, "\x1b[2K")
}

// spec §32.1 routes FormatJSON/FormatJSONL human presentation to Stderr
// exactly like FormatData — including the interactive live region when
// Stderr is a TTY. A plain-only stderr (no live region ever, even on an
// interactive terminal) would silently regress every JSON/JSONL CLI to the
// durable/plain renderer.
func TestFormatJSON_StderrGetsLiveRegionWhenInteractive(t *testing.T) {
	var stdout, stderr nopFlushWriter
	defer markWriterAsCharDevice(&stderr)()
	out := initRun(Config{
		isolated: true, title: "demo", format: formatJSON,
		stdout: &stdout, stderr: &stderr, visibilityDelay: delay(0),
	})
	out.Task("build").succeed("")
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if !hasLiveRegionExt(stderr.String()) {
		t.Fatalf("FormatJSON must open a live region on an interactive stderr:\n%q", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"object": "evo.run"`) {
		t.Fatalf("FormatJSON must still write the evo.run document to stdout:\n%s", stdout.String())
	}
}

func TestFormatJSONL_StderrGetsLiveRegionWhenInteractive(t *testing.T) {
	var stdout, stderr nopFlushWriter
	defer markWriterAsCharDevice(&stderr)()
	out := initRun(Config{
		isolated: true, title: "demo", format: formatJSONL,
		stdout: &stdout, stderr: &stderr, visibilityDelay: delay(0),
	})
	out.Task("build").succeed("")
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if !hasLiveRegionExt(stderr.String()) {
		t.Fatalf("FormatJSONL must open a live region on an interactive stderr:\n%q", stderr.String())
	}
}

func TestFormatJSON_RunReturnsResultUsableByWireEncoder(t *testing.T) {
	var stdout, stderr nopFlushWriter
	out := initRun(Config{
		isolated: true, title: "demo", format: formatJSON,
		stdout: &stdout, stderr: &stderr,
	})
	result := out.run(context.Background(), func(context.Context) error { return nil })
	if result.Conclusion.runID == "" {
		t.Fatalf("Result.Conclusion.RunID must be populated by Run/Finish")
	}
	if result.Conclusion.startedAt.IsZero() || result.Conclusion.finishedAt.IsZero() {
		t.Fatalf("Result.Conclusion.StartedAt/FinishedAt must be populated by Run/Finish")
	}
	if result.Conclusion.finishedAt.Before(result.Conclusion.startedAt) {
		t.Fatalf("FinishedAt (%v) must not precede StartedAt (%v)", result.Conclusion.finishedAt, result.Conclusion.startedAt)
	}
}
