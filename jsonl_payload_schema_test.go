package evo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
	"github.com/zachbornheimer/evident-output/v2/testkit/wireschema"
)

// TestJSONLDefinitionFinishedPayloadIsSchemaBound pins the two payloads a JSONL consumer reads to learn
// how a Task's callback ended. definition.finished says failed only for a callback that failed: an Exclude
// or Refuse ending is a decision, not a failure. verification.observed names the Task whose Verify ran,
// not a position. Every emitted line validates against schema/event.v2.json, which documents both.
func TestJSONLDefinitionFinishedPayloadIsSchemaBound(t *testing.T) {
	var stdout nopFlushWriter
	out := initRun(Config{Output: JSONL, Facades: Facades{Stdout: &stdout, Stderr: io.Discard}})
	endings := map[string]error{
		"ships":    nil,
		"breaks":   errors.New("disk full"),
		"declined": Exclude("not needed here"),
		"gated":    Refuse("no remote"),
	}
	names := map[string]string{}
	for name, ending := range endings {
		task := out.Task(name)
		names[task.id] = name
		task.Define(func(context.Context) error { return ending })
	}
	checked := out.Task("checked")
	names[checked.id] = "checked"
	checked.
		Verify(func(context.Context) (bool, error) { return false, nil }).
		Define(func(context.Context) error { return nil })
	_ = out.finish()

	schema := strictEventSchema(t)
	for line := range strings.SplitSeq(strings.TrimSpace(stdout.String()), "\n") {
		if err := wireschema.Validate(schema, []byte(line)); err != nil {
			t.Errorf("event does not conform to schema/event.v2.json:\n%v\n%s", err, line)
		}
	}

	wantFailed := map[string]bool{"ships": false, "breaks": true, "declined": false, "gated": false, "checked": false}
	finished := 0
	for line := range strings.SplitSeq(strings.TrimSpace(stdout.String()), "\n") {
		var e struct {
			Type     string         `json:"type"`
			EntityID string         `json:"entity_id"`
			Payload  map[string]any `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("event is not JSON: %v\n%s", err, line)
		}
		switch e.Type {
		case wire.EventDefinitionFinished:
			finished++
			name := names[e.EntityID]
			if got := e.Payload["failed"]; got != wantFailed[name] {
				t.Errorf("definition.finished for %q has failed=%v, want %v", name, got, wantFailed[name])
			}
		case wire.EventVerificationObserved:
			if got := e.Payload["name"]; got != "checked" {
				t.Errorf("verification.observed names %v, want the Task %q", got, "checked")
			}
		}
	}
	if finished != len(wantFailed) {
		t.Errorf("%d definition.finished events, want %d", finished, len(wantFailed))
	}
}

// TestEventSchemaRejectsMalformedPayloads shows the schema binds the payloads rather than describing them:
// a definition.finished without a boolean failed, or a verification.observed without its name, is invalid.
func TestEventSchemaRejectsMalformedPayloads(t *testing.T) {
	schema := strictEventSchema(t)
	line := func(eventType, payload string) []byte {
		return []byte(`{"object":"evo.event","schema_version":"` + wire.EventSchemaVersion +
			`","seq":1,"at":"2026-01-01T00:00:00Z","type":"` + eventType + `","payload":` + payload + `}`)
	}
	valid := map[string][]byte{
		"definition.finished":   line(wire.EventDefinitionFinished, `{"failed":false}`),
		"verification.observed": line(wire.EventVerificationObserved, `{"name":"checked","status":"satisfied"}`),
	}
	for name, doc := range valid {
		if err := wireschema.Validate(schema, doc); err != nil {
			t.Errorf("a well-formed %s payload is rejected: %v", name, err)
		}
	}
	invalid := map[string][]byte{
		"definition.finished without failed":     line(wire.EventDefinitionFinished, `{}`),
		"definition.finished with a string":      line(wire.EventDefinitionFinished, `{"failed":"yes"}`),
		"definition.finished with a stray field": line(wire.EventDefinitionFinished, `{"failed":true,"why":"x"}`),
		"verification.observed without a name":   line(wire.EventVerificationObserved, `{"status":"satisfied"}`),
	}
	for name, doc := range invalid {
		if err := wireschema.Validate(schema, doc); err == nil {
			t.Errorf("%s validates; the schema must reject it", name)
		}
	}
}

// strictEventSchema is schema/event.v2.json with unknown payload fields rejected.
func strictEventSchema(t *testing.T) []byte {
	t.Helper()
	schema, err := os.ReadFile("schema/event.v2.json")
	if err != nil {
		t.Fatalf("read schema/event.v2.json: %v", err)
	}
	strict, err := wireschema.Strict(schema)
	if err != nil {
		t.Fatalf("wireschema.Strict(event.v2.json): %v", err)
	}
	return strict
}
