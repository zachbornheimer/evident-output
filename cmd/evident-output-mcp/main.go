// Command evident-output-mcp is the stdio MCP server.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/agent/catalog"
	"github.com/zachbornheimer/evident-output/agent/preview"
	"github.com/zachbornheimer/evident-output/agent/review"
	"github.com/zachbornheimer/evident-output/agent/rules"
)

// Version is injected at build time.
var Version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Fprintf(os.Stderr, "evident-output-mcp %s\n", Version)
		os.Exit(0)
	}
	fmt.Fprintf(os.Stderr, "evident-output-mcp %s starting (stdio)\n", Version)
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
		switch method {
		case "initialize":
			writeRPC(id, map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]any{
					"tools":     map[string]any{},
					"resources": map[string]any{},
				},
				"serverInfo": map[string]any{"name": "evident-output-mcp", "version": Version},
			})
		case "tools/list":
			writeRPC(id, map[string]any{"tools": toolList()})
		case "tools/call":
			handleToolCall(id, req)
		case "resources/list":
			writeRPC(id, map[string]any{
				"resources": []map[string]any{
					{"uri": "evident-output://guides/common-api", "name": "common-api", "mimeType": "text/plain"},
					{"uri": "evident-output://rules/API-006", "name": "API-006", "mimeType": "application/json"},
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
	obj := map[string]any{"type": "object"}
	return []map[string]any{
		{"name": "evident_output.list_guides", "description": "List guidance catalog entries", "inputSchema": obj},
		{"name": "evident_output.get_guidance", "description": "Retrieve guidance sections by id", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
		}},
		{"name": "evident_output.review", "description": "Review Go source for evo misuse", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source": map[string]any{"type": "string"},
				"file":   map[string]any{"type": "string"},
			},
		}},
		{"name": "evident_output.preview", "description": "Preview plain profiles for a declarative scene", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"subject": map[string]any{"type": "string"},
				"item":    map[string]any{"type": "string"},
				"state":   map[string]any{"type": "string"},
			},
		}},
		{"name": "evident_output.explain", "description": "Explain a stable rule ID", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"rule_id": map[string]any{"type": "string"},
			},
		}},
	}
}

func handleToolCall(id any, req map[string]any) {
	params, _ := req["params"].(map[string]any)
	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)
	if args == nil {
		args = map[string]any{}
	}
	switch name {
	case "evident_output.list_guides":
		useCase, _ := args["use_case"].(string)
		guides := catalog.Filter(useCase)
		writeRPC(id, map[string]any{
			"content":           []map[string]any{{"type": "text", "text": fmt.Sprintf("%d guides", len(guides))}},
			"structuredContent": map[string]any{"guides": guides},
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
		writeRPC(id, map[string]any{
			"content":           []map[string]any{{"type": "text", "text": fmt.Sprintf("found=%d missing=%d", len(found), len(missing))}},
			"structuredContent": map[string]any{"guides": found, "missing": missing},
		})
	case "evident_output.explain":
		ruleID, _ := args["rule_id"].(string)
		if r, ok := rules.Explain(ruleID); ok {
			writeRPC(id, map[string]any{
				"content":           []map[string]any{{"type": "text", "text": r.Invariant}},
				"structuredContent": r,
			})
			return
		}
		writeRPC(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": "unknown rule"}},
			"isError": true,
		})
	case "evident_output.review":
		src, _ := args["source"].(string)
		file, _ := args["file"].(string)
		if file == "" {
			file = "input.go"
		}
		res := review.GoSource(file, src)
		writeRPC(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("findings=%d recheck=%v", len(res.Findings), res.RecheckRequired)}},
			"structuredContent": map[string]any{
				"recheck_required": res.RecheckRequired,
				"findings":         res.Findings,
			},
		})
	case "evident_output.preview":
		subject, _ := args["subject"].(string)
		item, _ := args["item"].(string)
		state, _ := args["state"].(string)
		if subject == "" {
			subject = "demo"
		}
		if item == "" {
			item = "status"
		}
		var buf bytes.Buffer
		out := evo.For(subject, evo.To(&buf), evo.Plain(), evo.NoColor())
		it := out.Item(item)
		switch state {
		case "blocked":
			it.Block("blocked for demo")
		case "failed":
			it.Fail("failed for demo")
		default:
			it.OK()
		}
		_ = out.Finish()
		snap := out.Snapshot()
		profiles := preview.DefaultProfiles(snap)
		writeRPC(id, map[string]any{
			"content":           []map[string]any{{"type": "text", "text": fmt.Sprintf("%d profiles", len(profiles))}},
			"structuredContent": map[string]any{"profiles": profiles},
		})
	default:
		writeRPC(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": "unknown tool"}},
			"isError": true,
		})
	}
}

func handleResourceRead(id any, req map[string]any) {
	params, _ := req["params"].(map[string]any)
	uri, _ := params["uri"].(string)
	switch {
	case len(uri) > len("evident-output://guides/") && uri[:len("evident-output://guides/")] == "evident-output://guides/":
		gid := uri[len("evident-output://guides/"):]
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
