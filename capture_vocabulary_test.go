package evo_test

import (
	"fmt"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestCaptureVocabulary_RetainsProcessOutputUnderCaptureNames proves the
// retained stdout/stderr sink is spelled Capture in 1.1 (vocabulary freeze:
// Evidence means only satisfaction proof). Each Capture spelling must carry
// the behavior the removed Evidence* spelling carried: CaptureOption knobs
// bound the ring, CaptureStream identifies the per-stream writers.
func TestCaptureVocabulary_RetainsProcessOutputUnderCaptureNames(t *testing.T) {
	out := evo.Init(evo.Config{Title: "capture", Stdout: io.Discard, Stderr: io.Discard})
	task := out.Task("build module")

	opts := []evo.CaptureOption{evo.KeepLastLines(2), evo.MaxCaptureBytes(1 << 10)}
	captured := task.CaptureForTest(opts...)
	for i := range 5 {
		_, _ = fmt.Fprintf(captured.Stderr(), "line %d\n", i)
	}
	_ = captured.Close()

	text := captured.Text()
	if strings.Contains(text, "line 0") || !strings.Contains(text, "line 4") {
		t.Fatalf("KeepLastLines(2) must retain only the tail, got %q", text)
	}
	streams := []evo.CaptureStream{evo.CaptureStreamCombined, evo.CaptureStreamStdout, evo.CaptureStreamStderr}
	if streams[0] == streams[1] || streams[0] == streams[2] || streams[1] == streams[2] {
		t.Fatalf("CaptureStream values must be pairwise distinct, got %v", streams)
	}
	succeed(task)
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}
