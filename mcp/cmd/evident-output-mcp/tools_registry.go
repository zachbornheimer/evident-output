package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"
)

func toolList() []map[string]any {
	tools := []map[string]any{
		{"name": "evident_output_list_sections", "description": "List the full docs corpus servable via evident_output_get_documentation (reference, development, MCP wiring, adoption ladder, per-concept guides)", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":       map[string]any{"type": "string"},
				"deadline_ms": map[string]any{"type": "integer"},
			},
		}},
		{"name": "evident_output_get_documentation", "description": "Retrieve one or more documentation sections by id (see evident_output_list_sections)", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ids":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"deadline_ms": map[string]any{"type": "integer"},
			},
		}},
		{"name": "evident_output_adopt_plan", "description": "Inventory non-evo CLI output (fmt.Print*/log.*/os.Stdout/spinner libs) under a directory and return a paged migration plan keyed to the adoption ladder", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"directory":   map[string]any{"type": "string"},
				"cursor":      map[string]any{"type": "string"},
				"limit":       map[string]any{"type": "integer"},
				"deadline_ms": map[string]any{"type": "integer"},
			},
			"required": []string{"directory"},
		}},
		{"name": "evident_output_review", "description": "Review Go source, a local directory, multi-file package, transcript, or structured JSON for evo misuse. MUST be used when migrating existing evo call sites (the autofixer, same role as svelte-autofixer): pass Go source or kind=directory, apply every suggestion, call again until zero findings", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source":          map[string]any{"type": "string"},
				"file":            map[string]any{"type": "string"},
				"directory":       map[string]any{"type": "string"},
				"kind":            map[string]any{"type": "string"},
				"files":           map[string]any{"type": "object"},
				"desired_version": map[string]any{"type": "string"},
				"deadline_ms":     map[string]any{"type": "integer"},
			},
		}},
		{"name": "evident_output_conformance", "description": "Answer whether a repo's Evo usage is current: what is stale, unsafe, mechanically upgradable, has explicit vs opaque provenance, or needs developer intent. Same inputs as evident_output_review (source/file or kind=directory); returns {target_version, findings[{rule,severity,file,line,summary,migration}]}", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source":         map[string]any{"type": "string"},
				"file":           map[string]any{"type": "string"},
				"directory":      map[string]any{"type": "string"},
				"kind":           map[string]any{"type": "string"},
				"target_version": map[string]any{"type": "string"},
				"deadline_ms":    map[string]any{"type": "integer"},
			},
		}},
		{"name": "evident_output_update", "description": "Install a matching evident-output-mcp binary for a go.mod pin (--directory) or a release tag (--version), then symlink into ~/.local/bin. Requires version XOR directory. After it returns, restart the MCP host.", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"version":     map[string]any{"type": "string"},
				"directory":   map[string]any{"type": "string"},
				"deadline_ms": map[string]any{"type": "integer"},
			},
		}},
		{"name": "evident_output_preview", "description": "Preview plain profiles for a declarative scene", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"subject":     map[string]any{"type": "string"},
				"item":        map[string]any{"type": "string"},
				"state":       map[string]any{"type": "string"},
				"debug":       map[string]any{"type": "string"},
				"deadline_ms": map[string]any{"type": "integer"},
			},
		}},
		{"name": "evident_output_explain", "description": "Explain a stable rule ID", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"rule_id":     map[string]any{"type": "string"},
				"deadline_ms": map[string]any{"type": "integer"},
			},
		}},
	}
	// MCP-042: enforce tool name rules at definition time.
	for _, t := range tools {
		name, _ := t["name"].(string)
		if !validToolName(name) {
			panic("invalid tool name: " + name)
		}
	}
	return tools
}

func validToolName(name string) bool {
	return len(name) > 0 && len(name) <= toolNameMaxLen && toolNameRE.MatchString(name)
}

// safeToolCall contains panics so one tool fault cannot kill the server (MCP-034).
func safeToolCall(id any, req map[string]any) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "tool panic: %v\n%s\n", r, debug.Stack())
			writeRPC(id, map[string]any{
				"content": []map[string]any{{"type": "text", "text": "internal tool error (panic contained)"}},
				"isError": true,
				"structuredContent": map[string]any{
					"schema": "evident_output.tool_error.v1",
					"code":   "panic_contained",
					"error":  fmt.Sprint(r),
				},
			})
		}
	}()
	handleToolCall(id, req)
}

// normalizeToolName maps legacy dotted names (evident_output_list_guides) to
// the advertised underscore form Grok and other hosts register cleanly.
func normalizeToolName(name string) string {
	if after, ok := strings.CutPrefix(name, "evident_output."); ok {
		return "evident_output_" + after
	}
	return name
}

func handleToolCall(id any, req map[string]any) {
	params, _ := req["params"].(map[string]any)
	name, _ := params["name"].(string)
	name = normalizeToolName(name)
	args, _ := params["arguments"].(map[string]any)
	if args == nil {
		args = map[string]any{}
	}
	if !validToolName(name) && name != "" {
		writeRPC(id, toolError("invalid tool name"))
		return
	}
	// MCP-034: fault injection point for tests (never set in production).
	if faultHook != nil {
		faultHook(name)
	}

	deadline := deadlineFromArgs(args)
	var cancelled atomic.Bool
	timer := time.AfterFunc(deadline, func() {
		cancelled.Store(true)
	})
	defer timer.Stop()
	// Soft deadline: tools are still synchronous (review is fast); we refuse to
	// return results after the deadline to avoid late-success races. Full
	// context-propagated cancellation is a follow-up for long reviews.

	// MCP-043: reject unknown argument fields per tool.
	if errMsg := validateArgs(name, args); errMsg != "" {
		writeRPC(id, toolError(errMsg))
		return
	}

	handler, ok := toolHandlers()[name]
	if !ok {
		writeRPC(id, toolError("unknown tool"))
		return
	}
	handler(id, args, &cancelled)
}

// toolHandler serves one MCP tool call. cancelled reports the call's soft
// deadline has passed; a handler must not return results after it.
type toolHandler func(id any, args map[string]any, cancelled *atomic.Bool)

// toolHandlers maps each advertised tool name to its handler: adding a
// tool is one entry here, one in toolList, and one in toolArgAllowlist.
func toolHandlers() map[string]toolHandler {
	return map[string]toolHandler{
		"evident_output_list_guides":       handleListGuides,
		"evident_output_get_guidance":      handleGetGuidance,
		"evident_output_explain":           handleExplain,
		"evident_output_list_sections":     handleListSections,
		"evident_output_get_documentation": handleGetDocumentation,
		"evident_output_adopt_plan":        handleAdoptPlan,
		"evident_output_review":            handleReview,
		"evident_output_conformance":       handleConformanceTool,
		"evident_output_update": func(id any, args map[string]any, _ *atomic.Bool) {
			handleUpdateTool(id, args)
		},
		"evident_output_preview": handlePreview,
	}
}

func toolError(msg string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": msg}},
		"isError": true,
		"structuredContent": map[string]any{
			"schema": "evident_output.tool_error.v1",
			"error":  msg,
		},
	}
}

// toolArgAllowlist is the argument-name registry validateArgs enforces —
// factored out so TestToolRegistryMatchesToolList can hold it against
// toolList() without duplicating the literal.
func toolArgAllowlist() map[string]map[string]bool {
	return map[string]map[string]bool{
		"evident_output_list_guides": {"use_case": true, "max_tokens": true, "deadline_ms": true},
		"evident_output_get_guidance": {
			"ids": true, "max_tokens": true, "deadline_ms": true,
		},
		"evident_output_review": {
			"source": true, "file": true, "directory": true, "kind": true, "files": true, "desired_version": true, "deadline_ms": true,
		},
		"evident_output_conformance": {
			"source": true, "file": true, "directory": true, "kind": true, "desired_version": true, "target_version": true, "deadline_ms": true,
		},
		"evident_output_update": {"version": true, "directory": true, "deadline_ms": true},
		"evident_output_preview": {
			"subject": true, "item": true, "state": true, "debug": true, "deadline_ms": true,
		},
		"evident_output_explain":       {"rule_id": true, "deadline_ms": true},
		"evident_output_list_sections": {"query": true, "deadline_ms": true},
		"evident_output_get_documentation": {
			"ids": true, "deadline_ms": true,
		},
		"evident_output_adopt_plan": {"directory": true, "cursor": true, "limit": true, "deadline_ms": true},
	}
}

func validateArgs(name string, args map[string]any) string {
	allowed := toolArgAllowlist()
	keys, ok := allowed[name]
	if !ok {
		return ""
	}
	for k := range args {
		if !keys[k] {
			return "unknown argument field: " + k
		}
	}
	return ""
}

func deadlineFromArgs(args map[string]any) time.Duration {
	ms := intFromArgs(args, "deadline_ms")
	if ms <= 0 {
		return defaultToolDeadline
	}
	return time.Duration(ms) * time.Millisecond
}

func intFromArgs(args map[string]any, key string) int {
	v, ok := args[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}
