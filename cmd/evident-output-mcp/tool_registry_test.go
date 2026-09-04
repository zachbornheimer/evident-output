package main

import "testing"

// TestToolRegistryMatchesToolList is the catalog↔registry invariant
// (internal/agent/catalog/catalog_test.go's TestCatalogRuleIDsResolve)
// extended over the tool surface: every tool toolList() advertises must
// have a validateArgs entry (or an unknown-field check silently no-ops for
// it), and validateArgs must never carry a stale entry for a tool that no
// longer exists. Either direction drifting is a dead end for an agent that
// discovers tools via tools/list.
func TestToolRegistryMatchesToolList(t *testing.T) {
	advertised := map[string]bool{}
	for _, tool := range toolList() {
		name, _ := tool["name"].(string)
		advertised[name] = true
	}

	allowed := toolArgAllowlist()
	for name := range advertised {
		if _, ok := allowed[name]; !ok {
			t.Errorf("tool %q is advertised by toolList but has no validateArgs entry", name)
		}
	}
	for name := range allowed {
		if !advertised[name] {
			t.Errorf("validateArgs has an entry for %q, which toolList no longer advertises", name)
		}
	}
}

// TestNewMCPToolsAdvertised pins the deliverable: the docs-serving and
// adoption tools this work order added must actually be reachable through
// tools/list, not just implemented in handleToolCall.
func TestNewMCPToolsAdvertised(t *testing.T) {
	want := []string{
		"evident_output_list_sections",
		"evident_output_get_documentation",
		"evident_output_adopt_plan",
	}
	advertised := map[string]bool{}
	for _, tool := range toolList() {
		name, _ := tool["name"].(string)
		advertised[name] = true
	}
	for _, name := range want {
		if !advertised[name] {
			t.Errorf("expected tool %q in toolList()", name)
		}
	}
}
