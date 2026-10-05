package main

import (
	"context"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestRunner_WriterOutputProjectsAsBoundedTail exercises the actual EVOR
// runner wiring: runCommand sends one child stream to Task.Writer, which must
// keep the complete evidence ring while the TTY projection shows only its
// bounded recent tail beneath the owner row.
func TestRunner_WriterOutputProjectsAsBoundedTail(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(100), testkit.Height(24), testkit.NoColor())
	zeroDelay := time.Duration(0)
	out := evo.Init(evo.Config{
		Title:           "evor test",
		Isolated:        true,
		Terminal:        screen,
		VisibilityDelay: &zeroDelay,
		Stdout:          nil,
		Stderr:          nil,
	})
	t.Cleanup(func() { _ = out.Close() })

	evidenceDir := t.TempDir()
	r := &runner{
		root:          ".",
		cfg:           Config{Title: "evor test"},
		evidence:      evidenceDir,
		heartbeat:     time.Hour,
		stream:        false,
		promptTailMax: defaultPromptTail,
	}
	task := out.Task("synthetic worker")
	_, err := r.runCommand(context.Background(), task, 1, "streaming child output", "work-01", Command{
		Name:    "synthetic worker",
		Command: []string{"sh", "-c", "for i in $(seq 1 20); do printf 'line %02d\\n' \"$i\"; done"},
		Timeout: "10s",
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	frame := screen.LatestLiveText()
	if !strings.Contains(frame, "synthetic worker") || !strings.Contains(frame, "line 20") {
		t.Fatalf("EVOR live frame lost the stable owner or newest output:\n%s", frame)
	}
	if !strings.Contains(frame, "line 15") || strings.Contains(frame, "line 14") {
		t.Fatalf("EVOR live frame did not show exactly the bounded recent tail:\n%s", frame)
	}
	if got := task.Capture().Text(); !strings.Contains(got, "line 01") || !strings.Contains(got, "line 20") {
		t.Fatalf("EVOR Task evidence did not retain the complete child stream:\n%s", got)
	}
}
