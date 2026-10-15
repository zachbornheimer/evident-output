package evo

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/testkit"
)

func fixedDebugClock() TestClock {
	return TestClock{T: time.Date(2026, 7, 27, 12, 4, 18, 219_000_000, time.UTC)}
}

// History mode (default): durable append-above, compact grammar with timestamp (§21.3.1).
func TestDebugHistory_AppendAboveLiveRegion(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := initRun(Config{Debug: slog.LevelDebug, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Clock: fixedDebugClock(), Terminal: screen}}, tuning{visibilityDelay: DelayForTest(0), color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("branches")
	task.Doing("comparing")
	out.debugForTest("opened repository", field{Key: "path", Value: "/work/repo"})
	succeed(task)
	_ = out.finish()

	var durable strings.Builder
	for _, op := range screen.Operations() {
		if op.Kind == "durable" {
			durable.WriteString(op.Text)
		}
	}
	if !strings.Contains(durable.String(), "12:04:18.219 [DEBUG] opened repository  path=/work/repo") {
		t.Fatalf("history line missing or wrong format:\nops=%#v\ndurable=%q", screen.Operations(), durable.String())
	}
	// Success: no diagnostic tail section in final.
	if strings.Contains(screen.FinalText(), "── diagnostics") {
		t.Fatalf("history mode must not emit pane diagnostics tail by default:\n%s", screen.FinalText())
	}
}

// Plain + history: still one stream, no double print (regression).
func TestDebugHistory_PlainStreamsOnce(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Debug: slog.LevelDebug, Output: Plain, Facades: Facades{Stdout: io.Discard, Stderr: &buf, Clock: fixedDebugClock()}}, tuning{color: colorNever})
	out.dropDiagnosticForTest()
	t.Cleanup(func() { _ = out.close() })
	out.debugForTest("cache warm", field{Key: "dir", Value: "/tmp/x"})
	if n := strings.Count(buf.String(), "[DEBUG] cache warm"); n != 1 {
		t.Fatalf("before Finish count=%d:\n%s", n, buf.String())
	}
	_ = out.finish()
	if n := strings.Count(buf.String(), "[DEBUG] cache warm"); n != 1 {
		t.Fatalf("after Finish count=%d:\n%s", n, buf.String())
	}
}
