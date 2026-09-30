package harness

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// RunDocument finishes out, encodes its result as the evo.run document, and
// returns it decoded generically.
func RunDocument(t *testing.T, out *evo.Output) map[string]any {
	t.Helper()
	_ = out.Finish()
	var buf bytes.Buffer
	if err := evo.WriteJSON(&buf, evo.Result{Conclusion: out.Conclusion()}); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("evo.run is not JSON: %v", err)
	}
	return doc
}

// Object reads a nested JSON object, failing the test when it is absent.
func Object(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	child, ok := parent[key].(map[string]any)
	if !ok {
		t.Fatalf("%q is not an object in %v", key, parent)
	}
	return child
}

// Objects reads a JSON array of objects, failing the test when it is absent.
func Objects(t *testing.T, parent map[string]any, key string) []map[string]any {
	t.Helper()
	raw, ok := parent[key].([]any)
	if !ok {
		t.Fatalf("%q is not an array in %v", key, parent)
	}
	items := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			items = append(items, m)
		}
	}
	return items
}

// JSONLines decodes a JSONL stream into one generic object per line.
func JSONLines(t *testing.T, stream string) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(stream), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %q is not JSON: %v", line, err)
		}
		lines = append(lines, m)
	}
	return lines
}
