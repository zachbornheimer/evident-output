package evo_test

import (
	"context"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

const (
	resultPayload = `{"artifact":"bin/app"}` + "\n"
	resultRowName = "compile sources"
)

// resultBody declares one visible Task and writes the data payload.
func resultBody(t *testing.T) evo.RunFunc {
	t.Helper()
	return func(_ context.Context, r *evo.Run) error {
		r.Task(resultRowName).Define(func(context.Context) error { return nil })
		if _, err := io.WriteString(r.ResultWriter(), resultPayload); err != nil {
			t.Errorf("write payload: %v", err)
		}
		return nil
	}
}

// ResultWriter is CONTRACT.md's "stdout data payload": a caller pipes stdout
// and gets the payload, while rows, bands and text stay on stderr.
func TestResultWriterReachesStdout(t *testing.T) {
	for _, mode := range contractOutputs {
		t.Run(outputName[mode], func(t *testing.T) {
			run := executeContract(context.Background(), t, evo.Config{Name: "build", Output: mode}, resultBody(t))

			if strings.Contains(run.Stderr, strings.TrimSpace(resultPayload)) {
				t.Fatalf("payload leaked into the presentation stream:\n%s", run.Stderr)
			}
			if !strings.Contains(run.Stderr, resultRowName) {
				t.Fatalf("rows vanished from the presentation stream:\n%s", run.Stderr)
			}
			switch mode {
			case evo.JSON:
				if got := runDocument(t, run.Stdout)["result"]; got != resultPayload {
					t.Fatalf("evo.run result = %q, want %q", got, resultPayload)
				}
			case evo.JSONL:
				events := eventsOfType(runEvents(t, run.Stdout), "result.written")
				if len(events) != 1 {
					t.Fatalf("result events = %d, want 1:\n%s", len(events), run.Stdout)
				}
				payload, _ := events[0]["payload"].(map[string]any)
				if payload["text"] != resultPayload {
					t.Fatalf("result event payload = %v, want text %q", payload, resultPayload)
				}
			default:
				if run.Stdout != resultPayload {
					t.Fatalf("stdout = %q, want exactly the payload %q", run.Stdout, resultPayload)
				}
			}
		})
	}
}
