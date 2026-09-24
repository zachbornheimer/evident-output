package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"slices"
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

// run11TopLevelKeys is every top-level key a 1.1 "evo.run" document has.
// A ^C'd run adds none: the cause stays in the human Explanation until a
// second cause exists to tell apart (DEC-CANCEL-007, deferred to ZYS-947).
var run11TopLevelKeys = []string{
	"data", "duration_ms", "evo_version", "exit_code", "finished_at", "mode",
	"object", "outcome", "run_id", "schema_version", "started_at",
}

// A 1.1 FormatJSON host interrupted by ^C gets the 1.1 document: outcome
// cancelled and no key 1.1 did not write.
func TestFormatJSON_SignalledRunKeepsThe11Document(t *testing.T) {
	body := runToStdout(t, FormatJSON, true)
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decode evo.run: %v\n%s", err, body)
	}
	if string(doc["outcome"]) != `"cancelled"` {
		t.Fatalf("outcome = %s, want \"cancelled\"\n%s", doc["outcome"], body)
	}
	for _, key := range slices.Sorted(maps.Keys(doc)) {
		if !slices.Contains(run11TopLevelKeys, key) {
			t.Errorf("signalled run's document has key %q, which 1.1 never wrote", key)
		}
	}
}

// A 1.1 FormatJSONL host interrupted by ^C gets the 1.1 run.finished
// payload: exactly outcome and exit_code.
func TestFormatJSONL_SignalledRunFinishedKeepsThe11Payload(t *testing.T) {
	for line := range strings.SplitSeq(strings.TrimSpace(string(runToStdout(t, FormatJSONL, true))), "\n") {
		var event struct {
			Type    string                     `json:"type"`
			Payload map[string]json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode event: %v\n%s", err, line)
		}
		if event.Type != "run.finished" {
			continue
		}
		if keys := slices.Sorted(maps.Keys(event.Payload)); !slices.Equal(keys, []string{"exit_code", "outcome"}) {
			t.Fatalf("run.finished payload keys = %v, want the 1.1 [exit_code outcome]", keys)
		}
		if string(event.Payload["outcome"]) != `"cancelled"` {
			t.Fatalf("run.finished outcome = %s, want \"cancelled\"", event.Payload["outcome"])
		}
		return
	}
	t.Fatal("no run.finished event in the JSONL stream")
}
