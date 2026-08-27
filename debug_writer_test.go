package evo_test

import (
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestDebugWriter_SplitsLinesAndSanitizes(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor())
	out := evo.NewWithOptions(evo.Terminal(screen), evo.VisibilityDelay(0), evo.DebugLevel(evo.Debug), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })

	w := out.DebugWriter()
	_, _ = w.Write([]byte("hello\x1b[31m\npartial"))
	_ = w.Close()
	_ = out.Finish()

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
