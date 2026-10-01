package driver_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/zachbornheimer/evident-output/eval/driver"
)

// fakeServer is an in-memory ToolServer.
type fakeServer struct {
	calls   []string
	replies map[string]driver.ToolResult
}

var mcpToolNames = []string{
	"evident_output_list_sections", "evident_output_get_documentation", "evident_output_review",
	"evident_output_explain", "evident_output_preview", "evident_output_adopt_plan", "evident_output_update",
}

func (f *fakeServer) Tools(context.Context) ([]driver.ToolDef, error) {
	var defs []driver.ToolDef
	for _, name := range mcpToolNames {
		defs = append(defs, driver.ToolDef{Name: name, Description: name, InputSchema: json.RawMessage(`{"type":"object"}`)})
	}
	return defs, nil
}

func (f *fakeServer) Call(_ context.Context, name string, _ json.RawMessage) (driver.ToolResult, error) {
	f.calls = append(f.calls, name)
	reply, ok := f.replies[name]
	if !ok {
		return driver.ToolResult{}, fmt.Errorf("fake server: no reply for %s", name)
	}
	return reply, nil
}

func (f *fakeServer) Close() error { return nil }

type memoryStore struct{ data map[string][]byte }

func (m *memoryStore) Get(key string) ([]byte, bool, error) {
	value, ok := m.data[key]
	return value, ok, nil
}

func (m *memoryStore) Put(key string, value []byte) error {
	if m.data == nil {
		m.data = map[string][]byte{}
	}
	m.data[key] = value
	return nil
}
