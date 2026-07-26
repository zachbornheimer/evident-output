// Command evident-output-mcp is the stdio MCP server.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/agent/catalog"
	"github.com/zachbornheimer/evident-output/agent/preview"
	"github.com/zachbornheimer/evident-output/agent/review"
	"github.com/zachbornheimer/evident-output/agent/rules"
)

// Version is injected at build time.
var Version = "dev"

// Supported protocol versions (MCP-041).
var supportedProtocols = map[string]bool{
	"2024-11-05": true,
	"2025-03-26": true,
}

const (
	defaultToolDeadline = 30 * time.Second
	toolNameMaxLen      = 64
)

var toolNameRE = regexp.MustCompile(`^[a-z][a-z0-9_.]{0,63}$`)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Fprintf(os.Stderr, "evident-output-mcp %s\n", Version)
		os.Exit(0)
	}
	fmt.Fprintf(os.Stderr, "evident-output-mcp %s starting (stdio)\n", Version)
	initialized := false
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req map[string]any
		if err := json.Unmarshal(line, &req); err != nil {
			writeRPCError(nil, -32700, "parse error")
			continue
		}
		method, _ := req["method"].(string)
		id := req["id"]
		// MCP-001: reject out-of-lifecycle tool/resource calls before initialize.
		if !initialized && method != "initialize" && method != "ping" {
			if id != nil {
				writeRPCError(id, -32002, "server not initialized; call initialize first")
			}
			continue
		}
		switch method {
		case "initialize":
			params, _ := req["params"].(map[string]any)
			clientProto, _ := params["protocolVersion"].(string)
			negotiated := "2024-11-05"
			if clientProto != "" {
				if !supportedProtocols[clientProto] {
					writeRPCError(id, -32602, "unsupported protocolVersion "+clientProto+"; supported: 2024-11-05, 2025-03-26")
					continue
				}
				negotiated = clientProto
			}
			initialized = true
			writeRPC(id, map[string]any{
				"protocolVersion": negotiated,
				"capabilities": map[string]any{
					"tools":     map[string]any{},
					"resources": map[string]any{},
				},
				"serverInfo": map[string]any{
					"name":            "evident-output-mcp",
					"version":         Version,
					"catalogChecksum": catalog.Checksum(),
				},
			})
		case "tools/list":
			writeRPC(id, map[string]any{"tools": toolList()})
		case "tools/call":
			safeToolCall(id, req)
		case "resources/list":
			writeRPC(id, map[string]any{
				"resources": []map[string]any{
					{"uri": "evident-output://guides/common-api", "name": "common-api", "mimeType": "text/plain"},
					{"uri": "evident-output://rules/API-006", "name": "API-006", "mimeType": "application/json"},
					{"uri": "evident-output://meta/catalog-checksum", "name": "catalog-checksum", "mimeType": "text/plain"},
				},
			})
		case "resources/read":
			handleResourceRead(id, req)
		case "notifications/initialized", "initialized", "ping":
			if id != nil {
				writeRPC(id, map[string]any{})
			}
		default:
			if id != nil {
				writeRPCError(id, -32601, "method not found: "+method)
			}
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "stdin: %v\n", err)
		os.Exit(1)
	}
}

func toolList() []map[string]any {
	tools := []map[string]any{
		{"name": "evident_output.list_guides", "description": "List guidance catalog entries", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"use_case":    map[string]any{"type": "string"},
				"max_tokens":  map[string]any{"type": "integer"},
				"deadline_ms": map[string]any{"type": "integer"},
			},
			"additionalProperties": false,
		}},
		{"name": "evident_output.get_guidance", "description": "Retrieve guidance sections by id", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ids":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"max_tokens":  map[string]any{"type": "integer"},
				"deadline_ms": map[string]any{"type": "integer"},
			},
			"additionalProperties": false,
		}},
		{"name": "evident_output.review", "description": "Review Go source, multi-file package, transcript, or structured JSON for evo misuse", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source":      map[string]any{"type": "string"},
				"file":        map[string]any{"type": "string"},
				"kind":        map[string]any{"type": "string"},
				"files":       map[string]any{"type": "object"},
				"deadline_ms": map[string]any{"type": "integer"},
			},
			"additionalProperties": false,
		}},
		{"name": "evident_output.preview", "description": "Preview plain profiles for a declarative scene", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"subject":     map[string]any{"type": "string"},
				"item":        map[string]any{"type": "string"},
				"state":       map[string]any{"type": "string"},
				"debug":       map[string]any{"type": "string"},
				"deadline_ms": map[string]any{"type": "integer"},
			},
			"additionalProperties": false,
		}},
		{"name": "evident_output.explain", "description": "Explain a stable rule ID", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"rule_id":     map[string]any{"type": "string"},
				"deadline_ms": map[string]any{"type": "integer"},
			},
			"additionalProperties": false,
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

func handleToolCall(id any, req map[string]any) {
	params, _ := req["params"].(map[string]any)
	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)
	if args == nil {
		args = map[string]any{}
	}
	if !validToolName(name) && name != "" {
		writeRPC(id, toolError("invalid tool name"))
		return
	}

	deadline := deadlineFromArgs(args)
	done := make(chan struct{})
	var cancelled bool
	timer := time.AfterFunc(deadline, func() {
		cancelled = true
	})
	defer timer.Stop()
	defer close(done)

	// MCP-043: reject unknown argument fields per tool.
	if errMsg := validateArgs(name, args); errMsg != "" {
		writeRPC(id, toolError(errMsg))
		return
	}

	switch name {
	case "evident_output.list_guides":
		useCase, _ := args["use_case"].(string)
		guides := catalog.Filter(useCase)
		maxTok := intFromArgs(args, "max_tokens")
		truncated := false
		if maxTok > 0 {
			guides, truncated = catalog.ApplyTokenBudget(guides, maxTok)
		}
		if cancelled {
			writeRPC(id, toolError("deadline exceeded"))
			return
		}
		text := fmt.Sprintf("%d guides", len(guides))
		if truncated {
			text += " (truncated to token budget)"
		}
		writeRPC(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"structuredContent": map[string]any{
				"schema":    "evident_output.guides.v1",
				"guides":    guides,
				"truncated": truncated,
				"checksum":  catalog.Checksum(),
			},
		})
	case "evident_output.get_guidance":
		var ids []string
		if raw, ok := args["ids"].([]any); ok {
			for _, v := range raw {
				if s, ok := v.(string); ok {
					ids = append(ids, s)
				}
			}
		}
		found, missing := catalog.Get(ids)
		maxTok := intFromArgs(args, "max_tokens")
		truncated := false
		if maxTok > 0 {
			found, truncated = catalog.ApplyTokenBudget(found, maxTok)
		}
		if cancelled {
			writeRPC(id, toolError("deadline exceeded"))
			return
		}
		text := fmt.Sprintf("found=%d missing=%d", len(found), len(missing))
		if truncated {
			text += " truncated"
		}
		writeRPC(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"structuredContent": map[string]any{
				"schema":    "evident_output.guidance.v1",
				"guides":    found,
				"missing":   missing,
				"truncated": truncated,
			},
		})
	case "evident_output.explain":
		ruleID, _ := args["rule_id"].(string)
		if r, ok := rules.Explain(ruleID); ok {
			writeRPC(id, map[string]any{
				"content": []map[string]any{{"type": "text", "text": r.Invariant}},
				"structuredContent": map[string]any{
					"schema": "evident_output.rule.v1",
					"rule":   r,
				},
			})
			return
		}
		writeRPC(id, toolError("unknown rule"))
	case "evident_output.review":
		src, _ := args["source"].(string)
		file, _ := args["file"].(string)
		kind, _ := args["kind"].(string)
		if file == "" {
			file = "input.go"
		}
		// MCP-036: remote paths unsupported — accept inlined content only.
		if isRemotePath(file) {
			writeRPC(id, toolError("remote path unsupported; pass source content only (MCP-036)"))
			return
		}
		var res review.Result
		switch kind {
		case "transcript":
			res = review.Transcript(file, src)
		case "json", "structured":
			res = review.StructuredDocument(file, []byte(src))
		case "package":
			// MCP-017: multi-file map via JSON object in source or single pair.
			files := map[string]string{file: src}
			if raw, ok := args["files"].(map[string]any); ok {
				files = map[string]string{}
				for k, v := range raw {
					if s, ok := v.(string); ok {
						files[k] = s
					}
				}
			}
			res = review.GoPackage(files)
		default:
			res = review.GoSource(file, src)
		}
		if cancelled {
			writeRPC(id, toolError("deadline exceeded"))
			return
		}
		text := fmt.Sprintf("findings=%d recheck=%v partial=%v", len(res.Findings), res.RecheckRequired, res.Partial)
		writeRPC(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"structuredContent": map[string]any{
				"schema":           "evident_output.review.v1",
				"recheck_required": res.RecheckRequired,
				"partial":          res.Partial,
				"findings":         res.Findings,
			},
		})
	case "evident_output.preview":
		subject, _ := args["subject"].(string)
		item, _ := args["item"].(string)
		state, _ := args["state"].(string)
		dbg, _ := args["debug"].(string)
		if subject == "" {
			subject = "demo"
		}
		if item == "" {
			item = "status"
		}
		var buf bytes.Buffer
		out := evo.For(subject, evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DebugLevel(evo.Debug))
		it := out.Item(item)
		switch state {
		case "blocked":
			it.Block("blocked for demo")
		case "failed":
			it.Fail("failed for demo")
		default:
			it.OK()
		}
		if dbg != "" {
			out.Debug(dbg)
		}
		_ = out.Finish()
		snap := out.Snapshot()
		profiles := preview.DefaultProfiles(snap)
		if cancelled {
			writeRPC(id, toolError("deadline exceeded"))
			return
		}
		writeRPC(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("%d profiles", len(profiles))}},
			"structuredContent": map[string]any{
				"schema":   "evident_output.preview.v1",
				"profiles": profiles,
				"plain":    buf.String(),
			},
		})
	default:
		writeRPC(id, toolError("unknown tool"))
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

func validateArgs(name string, args map[string]any) string {
	allowed := map[string]map[string]bool{
		"evident_output.list_guides": {"use_case": true, "max_tokens": true, "deadline_ms": true},
		"evident_output.get_guidance": {
			"ids": true, "max_tokens": true, "deadline_ms": true,
		},
		"evident_output.review": {
			"source": true, "file": true, "kind": true, "files": true, "deadline_ms": true,
		},
		"evident_output.preview": {
			"subject": true, "item": true, "state": true, "debug": true, "deadline_ms": true,
		},
		"evident_output.explain": {"rule_id": true, "deadline_ms": true},
	}
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

func handleResourceRead(id any, req map[string]any) {
	params, _ := req["params"].(map[string]any)
	uri, _ := params["uri"].(string)
	// SEC-014: reject path traversal in URIs.
	if containsTraversal(uri) {
		writeRPCError(id, -32002, "resource not found: traversal rejected")
		return
	}
	switch {
	case uri == "evident-output://meta/catalog-checksum":
		writeRPC(id, map[string]any{
			"contents": []map[string]any{{
				"uri": uri, "mimeType": "text/plain", "text": catalog.Checksum(),
			}},
		})
	case len(uri) > len("evident-output://guides/") && uri[:len("evident-output://guides/")] == "evident-output://guides/":
		gid := uri[len("evident-output://guides/"):]
		if containsTraversal(gid) || gid == "" {
			writeRPCError(id, -32002, "resource not found")
			return
		}
		found, _ := catalog.Get([]string{gid})
		if len(found) == 0 {
			writeRPCError(id, -32002, "resource not found")
			return
		}
		writeRPC(id, map[string]any{
			"contents": []map[string]any{{"uri": uri, "mimeType": "text/plain", "text": found[0].Body}},
		})
	case len(uri) > len("evident-output://rules/") && uri[:len("evident-output://rules/")] == "evident-output://rules/":
		rid := uri[len("evident-output://rules/"):]
		if containsTraversal(rid) {
			writeRPCError(id, -32002, "resource not found")
			return
		}
		if r, ok := rules.Explain(rid); ok {
			b, _ := json.Marshal(r)
			writeRPC(id, map[string]any{
				"contents": []map[string]any{{"uri": uri, "mimeType": "application/json", "text": string(b)}},
			})
			return
		}
		writeRPCError(id, -32002, "resource not found")
	default:
		writeRPCError(id, -32002, "resource not found")
	}
}

func containsTraversal(s string) bool {
	return bytes.Contains([]byte(s), []byte("..")) || bytes.Contains([]byte(s), []byte("\\"))
}

// isRemotePath reports unsupported remote fetch schemes (MCP-036).
func isRemotePath(p string) bool {
	lower := strings.ToLower(p)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "git+") ||
		strings.HasPrefix(lower, "ssh://") ||
		(strings.Contains(lower, "://") && !strings.HasPrefix(lower, "file://"))
}

func writeRPC(id any, result any) {
	msg := map[string]any{"jsonrpc": "2.0", "id": id, "result": result}
	_ = json.NewEncoder(os.Stdout).Encode(msg)
}

func writeRPCError(id any, code int, message string) {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": message},
	}
	_ = json.NewEncoder(os.Stdout).Encode(msg)
}
