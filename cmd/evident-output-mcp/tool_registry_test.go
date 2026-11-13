package main

import "testing"

var advertisedToolNames = []string{
	"evident_output_list_sections",
	"evident_output_get_documentation",
	"evident_output_adopt_plan",
	"evident_output_review",
	"evident_output_preview",
	"evident_output_explain",
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

// TestNewMCPToolsAdvertised pins tools/list to exactly the six advertised names.
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
