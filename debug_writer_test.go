package evo

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/testkit"
)

func TestDebugWriter_SplitsLinesAndSanitizes(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor())
	out := initRun(Config{Debug: slog.LevelDebug, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Terminal: screen}}, tuning{visibilityDelay: DelayForTest(0), color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	w := out.debugWriterForTest()
	_, _ = w.Write([]byte("hello\x1b[31m\npartial"))
	_ = w.Close()
	_ = out.finish()

	var durable []string
	for _, op := range screen.Operations() {
		if op.Kind == "durable" {
			durable = append(durable, op.Text)
		}
	}
	// Debug lines must neutralize user ESC; do not inspect colored finals.
	joined := strings.Join(durable, "\n")
	if strings.Contains(joined, "\x1b") {
		t.Fatalf("ESC leaked into debug lines: %q", joined)
	}
	if !strings.Contains(joined, "hello") {
		t.Fatalf("missing debug content: %q", joined)
	}
}
