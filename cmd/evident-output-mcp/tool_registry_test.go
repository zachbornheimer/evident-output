package main

import (
	"encoding/json"
	"strings"
	"testing"
)

var advertisedToolNames = []string{
	"evident_output_list_sections",
	"evident_output_get_documentation",
	"evident_output_adopt_plan",
	"evident_output_review",
	"evident_output_preview",
	"evident_output_explain",
	"evident_output_update",
}

var callableAliasNames = []string{
	"evident_output_list_guides",
	"evident_output_get_guidance",
}

func advertisedSet() map[string]bool {
	out := map[string]bool{}
	for _, tool := range toolList() {
		name, _ := tool["name"].(string)
		out[name] = true
	}
	return out
}

// TestToolRegistryMatchesToolList splits advertised (tools/list) from
// callable (validateArgs / tools/call). Advertised tools must have an
// allowlist entry. Aliases may be callable without being advertised.
func TestToolRegistryMatchesToolList(t *testing.T) {
	advertised := advertisedSet()
	allowed := toolArgAllowlist()
	for name := range advertised {
		if _, ok := allowed[name]; !ok {
			t.Errorf("tool %q is advertised by toolList but has no validateArgs entry", name)
		}
	}
	for _, name := range callableAliasNames {
		if _, ok := allowed[name]; !ok {
			t.Errorf("alias %q must remain callable (validateArgs entry)", name)
		}
		if advertised[name] {
			t.Errorf("alias %q must not appear in tools/list", name)
		}
	}
	for name := range allowed {
		if advertised[name] {
			continue
		}
		isAlias := false
		for _, alias := range callableAliasNames {
			if name == alias {
				isAlias = true
				break
			}
		}
		if !isAlias {
			t.Errorf("validateArgs has an entry for %q, which is neither advertised nor an alias", name)
		}
	}
}

// TestNewMCPToolsAdvertised pins tools/list to exactly the advertised names.
func TestNewMCPToolsAdvertised(t *testing.T) {
	advertised := advertisedSet()
	if len(advertised) != len(advertisedToolNames) {
		t.Errorf("toolList advertised %d tools, want %d: %v", len(advertised), len(advertisedToolNames), advertised)
	}
	for _, name := range advertisedToolNames {
		if !advertised[name] {
			t.Errorf("expected tool %q in toolList()", name)
		}
	}
	for _, name := range callableAliasNames {
		if advertised[name] {
			t.Errorf("alias %q must not be advertised by toolList()", name)
		}
	}
}

func TestMCP_ToolsListIncludesUpdateWithSchema(t *testing.T) {
	bin := buildMCP(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	var update map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		result, _ := msg["result"].(map[string]any)
		tools, _ := result["tools"].([]any)
		for _, raw := range tools {
			tm, _ := raw.(map[string]any)
			if tm["name"] == "evident_output_update" {
				update = tm
			}
		}
	}
	if update == nil {
		t.Fatalf("tools/list missing evident_output_update: %s", out)
	}
	schema, _ := update["inputSchema"].(map[string]any)
	if len(schema) == 0 {
		t.Fatalf("evident_output_update missing inputSchema: %v", update)
	}
	if schema["type"] != "object" {
		t.Fatalf("inputSchema.type=%v", schema["type"])
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["version"]; !ok {
		t.Fatalf("inputSchema missing version: %v", schema)
	}
	if _, ok := props["directory"]; !ok {
		t.Fatalf("inputSchema missing directory: %v", schema)
	}
	listed := listedToolNames(t, out)
	if len(listed) != 7 {
		t.Fatalf("advertised %d tools, want 7: %v", len(listed), listed)
	}
}
