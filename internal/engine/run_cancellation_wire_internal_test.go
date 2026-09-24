package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// runToStdout runs one Task under format, sending one ^C from the run
// callback while the Task runs when interrupted is set (the 1.1 signal
// window), and returns what the run wrote to Stdout.
func runToStdout(t *testing.T, format Format, interrupted bool) []byte {
	t.Helper()
	interrupt := sendOneSignal(t)
	var stdout bytes.Buffer
	out := Init(Config{Isolated: true, Plain: true, Format: format, Stdout: &stdout, Stderr: io.Discard})
	out.Run(context.Background(), func(ctx context.Context) error {
		started := make(chan struct{})
		out.Task("install agent").Define(func(taskCtx context.Context) error {
			close(started)
			if interrupted {
				<-taskCtx.Done()
			}
			return nil
		})
		if interrupted {
			<-started
			interrupt()
			<-ctx.Done()
		}
		return nil
	})
	return stdout.Bytes()
}

// cancellationRecord is the "cancellation" object both wire projections
// carry on a cancelled run.
type cancellationRecord struct {
	Cause string `json:"cause"`
}

// DEC-CANCEL-007: a ^C'd run's "evo.run" document names its cause as a
// stable code, so a machine consumer never parses "by user".
func TestFormatJSON_SignalledRunNamesUserCause(t *testing.T) {
	var doc struct {
		Outcome      string              `json:"outcome"`
		Cancellation *cancellationRecord `json:"cancellation"`
	}
	body := runToStdout(t, FormatJSON, true)
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decode evo.run: %v\n%s", err, body)
	}
	if doc.Outcome != "cancelled" || doc.Cancellation == nil || doc.Cancellation.Cause != "user" {
		t.Fatalf("document outcome=%q cancellation=%+v, want cancelled with cause \"user\"", doc.Outcome, doc.Cancellation)
	}
}

// A completed run's document has no cancellation key at all.
func TestFormatJSON_CompletedRunHasNoCancellation(t *testing.T) {
	if body := runToStdout(t, FormatJSON, false); strings.Contains(string(body), `"cancellation"`) {
		t.Fatalf("completed run's document names a cancellation:\n%s", body)
	}
}

// The JSONL run.finished event carries the same record as the final
// document, so the two machine projections never disagree.
func TestFormatJSONL_RunFinishedNamesUserCause(t *testing.T) {
	for line := range strings.SplitSeq(strings.TrimSpace(string(runToStdout(t, FormatJSONL, true))), "\n") {
		var event struct {
			Type    string `json:"type"`
			Payload struct {
				Cancellation *cancellationRecord `json:"cancellation"`
			} `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode event: %v\n%s", err, line)
		}
		if event.Type != "run.finished" {
			continue
		}
		if c := event.Payload.Cancellation; c == nil || c.Cause != "user" {
			t.Fatalf("run.finished cancellation = %+v, want cause \"user\"", c)
		}
		return
	}
	t.Fatal("no run.finished event in the JSONL stream")
}
