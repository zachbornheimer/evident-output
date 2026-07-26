package evo_test

import (
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestDebugWriter_SplitsLinesAndSanitizes(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor())
	out := evo.New(evo.Terminal(screen), evo.DebugLevel(evo.Debug))
	t.Cleanup(func() { _ = out.Close() })

	w := out.DebugWriter()
	_, _ = w.Write([]byte("hello\x1b[31m\npartial"))
	_ = w.Close()
	_ = out.Finish()

	var durable []string
	for _, op := range screen.Operations() {
		if op.Kind == "durable" || strings.Contains(op.Text, "hello") {
			durable = append(durable, op.Text)
		}
	}
	// At least one debug line without ESC.
	joined := strings.Join(durable, "\n")
	if strings.Contains(joined, "\x1b") {
		t.Fatalf("ESC leaked: %q", joined)
	}
}
