package machine

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// TestEncodeJSON_StampsSchemaVersionAndFlattensNestedCollections proves the
// wire 0.4 invariant this package exists to hold: every JSONDocument names
// its own schema_version, and a nested container's tasks land in the flat
// TaskCollections/Tasks lists instead of being dropped when a Group nests
// another Group.
func TestEncodeJSON_StampsSchemaVersionAndFlattensNestedCollections(t *testing.T) {
	inner := core.TaskSnapshot{ID: "t2", Name: "inner-task", State: core.Done}
	nested := core.TasksSnapshot{
		ID:    "col2",
		Name:  "nested",
		State: core.Done,
		Tasks: []core.TaskSnapshot{inner},
	}
	outer := core.TasksSnapshot{
		ID:          "col1",
		Name:        "outer",
		State:       core.Done,
		Collections: []core.TasksSnapshot{nested},
	}
	snap := core.Snapshot{
		OutputID:    "out-1",
		Subject:     "test run",
		Collections: []core.TasksSnapshot{outer},
	}

	raw, err := EncodeJSON(snap)
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}

	var doc JSONDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc.SchemaVersion != JSONSchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", doc.SchemaVersion, JSONSchemaVersion)
	}
	if len(doc.TaskCollections) != 2 {
		t.Fatalf("TaskCollections = %d, want 2 (outer + nested)", len(doc.TaskCollections))
	}
	if len(doc.Tasks) != 1 || doc.Tasks[0].ID != "t2" {
		t.Fatalf("Tasks = %v, want [t2] (the nested collection's task, not dropped)", doc.Tasks)
	}
}

// TestEncodeJSONL_EncodesOneEventPerLine proves EncodeJSONL emits exactly
// one JSON object per input event, each stamped with the event schema
// version, so a machine consumer can split on newlines without parsing the
// whole payload first.
func TestEncodeJSONL_EncodesOneEventPerLine(t *testing.T) {
	events := []core.Event{
		{Sequence: 1, Type: "task_started", EntityID: "t1", Timestamp: time.Unix(0, 0)},
		{Sequence: 2, Type: "task_done", EntityID: "t1", Timestamp: time.Unix(1, 0)},
	}

	raw, err := EncodeJSONL(events)
	if err != nil {
		t.Fatalf("EncodeJSONL: %v", err)
	}

	lines := splitLines(raw)
	if len(lines) != len(events) {
		t.Fatalf("got %d lines, want %d", len(lines), len(events))
	}
	for i, line := range lines {
		var ev EventJSON
		if err := json.Unmarshal(line, &ev); err != nil {
			t.Fatalf("line %d: unmarshal: %v", i, err)
		}
		if ev.SchemaVersion != core.EventSchemaVersion {
			t.Errorf("line %d: SchemaVersion = %q, want %q", i, ev.SchemaVersion, core.EventSchemaVersion)
		}
		if ev.Sequence != events[i].Sequence {
			t.Errorf("line %d: Sequence = %d, want %d", i, ev.Sequence, events[i].Sequence)
		}
	}
}

func splitLines(raw []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range raw {
		if b == '\n' {
			if i > start {
				lines = append(lines, raw[start:i])
			}
			start = i + 1
		}
	}
	return lines
}
