package evo_test

import (
	"context"
	"encoding/base64"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// binaryPayload is not valid UTF-8, so a JSON string or a text field cannot carry it.
const binaryPayload = "ok \xff\xfe bin\n"

// ResultWriter hands the caller's bytes to stdout exactly: Human and Plain pass them through, and JSON and
// JSONL carry them in a base64 field when they are not valid UTF-8.
func TestResultWriterBytesSurviveMachineOutput(t *testing.T) {
	body := func(_ context.Context, r *evo.Run) error {
		if _, err := r.ResultWriter().Write([]byte(binaryPayload)); err != nil {
			t.Errorf("write payload: %v", err)
		}
		return nil
	}
	for _, mode := range contractOutputs {
		t.Run(outputName[mode], func(t *testing.T) {
			run := executeContract(context.Background(), t, evo.Config{Name: "build", Output: mode}, body)

			switch mode {
			case evo.JSON:
				doc := runDocument(t, run.Stdout)
				if got := decodeBase64Field(t, doc, "result_base64"); got != binaryPayload {
					t.Fatalf("result_base64 decodes to %q, want %q", got, binaryPayload)
				}
				if _, textual := doc["result"]; textual {
					t.Fatalf("result carries text for a payload that is not valid UTF-8: %v", doc["result"])
				}
			case evo.JSONL:
				events := eventsOfType(runEvents(t, run.Stdout), "result.written")
				if len(events) != 1 {
					t.Fatalf("result events = %d, want 1:\n%s", len(events), run.Stdout)
				}
				payload, _ := events[0]["payload"].(map[string]any)
				if got := decodeBase64Field(t, payload, "base64"); got != binaryPayload {
					t.Fatalf("payload base64 decodes to %q, want %q", got, binaryPayload)
				}
				if _, textual := payload["text"]; textual {
					t.Fatalf("payload carries text for bytes that are not valid UTF-8: %v", payload["text"])
				}
			default:
				if run.Stdout != binaryPayload {
					t.Fatalf("stdout = %q, want exactly %q", run.Stdout, binaryPayload)
				}
			}
		})
	}
}

func decodeBase64Field(t *testing.T, object map[string]any, field string) string {
	t.Helper()
	encoded, ok := object[field].(string)
	if !ok {
		t.Fatalf("%s missing or not a string in %v", field, object)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("%s is not base64: %v", field, err)
	}
	return string(raw)
}
