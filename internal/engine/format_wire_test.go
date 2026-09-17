package engine

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
		want    Format
		wantErr bool
	}{
		{"human", FormatHuman, false},
		{"  human  ", FormatHuman, false},
		{"HUMAN", FormatHuman, false},
		{"data", FormatData, false},
		{"external", FormatExternal, false},
		{"json", FormatJSON, false},
		{"jsonl", FormatJSONL, false},
		{"JSONL", FormatJSONL, false},
		{"yaml", 0, true},
		{"", 0, true},
	}
	for _, tc := range cases {
		got, err := ParseFormat(tc.in)
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
	out := Init(Config{
		Isolated: true, Title: "demo", Format: FormatJSON,
		Stdout: &stdout, Stderr: &stderr,
	})
	out.Task("build").Done()
	if err := out.Finish(); err != nil {
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
	out := Init(Config{
		Isolated: true, Title: "demo", Format: FormatJSON,
		Stdout: erroringWriter{}, Stderr: &stderr,
	})
	result := out.Run(context.Background(), func(context.Context) error {
		out.Task("build").Done()
		return nil
	})
	if result.ExitCode() != ExitFailed {
		t.Fatalf("Result.ExitCode() = %d, want ExitFailed (%d) after a stdout write failure", result.ExitCode(), ExitFailed)
	}
}

func TestFormatJSONL_StdoutStreamsEventLinesStderrHasHuman(t *testing.T) {
	var stdout, stderr nopFlushWriter
	out := Init(Config{
		Isolated: true, Title: "demo", Format: FormatJSONL,
		Stdout: &stdout, Stderr: &stderr,
	})
	out.Task("build").Done()
	if err := out.Finish(); err != nil {
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

// hasLiveRegion reports whether s carries the ANSI live-region markers
// terminal.ANSI.WriteLive emits (cursor-hide / erase-line) — the same test
// used by the root package's env_output_test.go to detect an armed live
// surface without depending on a real pty.
func hasLiveRegion(s string) bool {
	return strings.Contains(s, "\x1b[?25") || strings.Contains(s, "\x1b[2K")
}

// spec §32.1 routes FormatJSON/FormatJSONL human presentation to Stderr
// exactly like FormatData — including the interactive live region when
// Stderr is a TTY. A plain-only stderr (no live region ever, even on an
// interactive terminal) would silently regress every JSON/JSONL CLI to the
// durable/plain renderer.
func TestFormatJSON_StderrGetsLiveRegionWhenInteractive(t *testing.T) {
	var stdout, stderr nopFlushWriter
	defer MarkWriterAsCharDevice(&stderr)()
	out := Init(Config{
		Isolated: true, Title: "demo", Format: FormatJSON,
		Stdout: &stdout, Stderr: &stderr, VisibilityDelay: Delay(0),
	})
	out.Task("build").Done()
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if !hasLiveRegion(stderr.String()) {
		t.Fatalf("FormatJSON must open a live region on an interactive stderr:\n%q", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"object": "evo.run"`) {
		t.Fatalf("FormatJSON must still write the evo.run document to stdout:\n%s", stdout.String())
	}
}

func TestFormatJSONL_StderrGetsLiveRegionWhenInteractive(t *testing.T) {
	var stdout, stderr nopFlushWriter
	defer MarkWriterAsCharDevice(&stderr)()
	out := Init(Config{
		Isolated: true, Title: "demo", Format: FormatJSONL,
		Stdout: &stdout, Stderr: &stderr, VisibilityDelay: Delay(0),
	})
	out.Task("build").Done()
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if !hasLiveRegion(stderr.String()) {
		t.Fatalf("FormatJSONL must open a live region on an interactive stderr:\n%q", stderr.String())
	}
}

func TestFormatJSON_RunReturnsResultUsableByWireEncoder(t *testing.T) {
	var stdout, stderr nopFlushWriter
	out := Init(Config{
		Isolated: true, Title: "demo", Format: FormatJSON,
		Stdout: &stdout, Stderr: &stderr,
	})
	result := out.Run(context.Background(), func(context.Context) error { return nil })
	if result.Conclusion.RunID == "" {
		t.Fatalf("Result.Conclusion.RunID must be populated by Run/Finish")
	}
	if result.Conclusion.StartedAt.IsZero() || result.Conclusion.FinishedAt.IsZero() {
		t.Fatalf("Result.Conclusion.StartedAt/FinishedAt must be populated by Run/Finish")
	}
	if result.Conclusion.FinishedAt.Before(result.Conclusion.StartedAt) {
		t.Fatalf("FinishedAt (%v) must not precede StartedAt (%v)", result.Conclusion.FinishedAt, result.Conclusion.StartedAt)
	}
}
