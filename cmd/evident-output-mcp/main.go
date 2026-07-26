// Command evident-output-mcp is the stdio MCP server (v0.6 skeleton).
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/zachbornheimer/evident-output/agent/review"
)

// Version is injected at build time.
var Version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Fprintf(os.Stderr, "evident-output-mcp %s\n", Version)
		os.Exit(0)
	}
	// Stdio MCP: read JSON-RPC lines from stdin, write only protocol frames to stdout.
	// Logs go to stderr only (MCP-003/004).
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
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "evident-output-mcp", "version": Version},
			})
		case "tools/list":
			writeRPC(id, map[string]any{
				"tools": []map[string]any{
					{"name": "evident_output.list_guides", "description": "List guidance catalog entries", "inputSchema": map[string]any{"type": "object"}},
					{"name": "evident_output.get_guidance", "description": "Retrieve guidance sections", "inputSchema": map[string]any{"type": "object"}},
					{"name": "evident_output.review", "description": "Review Go source or transcripts", "inputSchema": map[string]any{"type": "object"}},
					{"name": "evident_output.preview", "description": "Preview terminal profiles", "inputSchema": map[string]any{"type": "object"}},
					{"name": "evident_output.explain", "description": "Explain a rule ID", "inputSchema": map[string]any{"type": "object"}},
				},
			})
		case "tools/call":
			handleToolCall(id, req)

		case "notifications/initialized", "initialized":
			// no response for notifications
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

func writeRPC(id any, result any) {
	msg := map[string]any{"jsonrpc": "2.0", "id": id, "result": result}
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(msg)
}

func writeRPCError(id any, code int, message string) {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": message},
	}
	_ = json.NewEncoder(os.Stdout).Encode(msg)
}

func handleToolCall(id any, req map[string]any) {
	params, _ := req["params"].(map[string]any)
	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)
	switch name {
	case "evident_output.list_guides":
		writeRPC(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": "guides: common-api, tasks, streams, security"}},
			"structuredContent": map[string]any{
				"guides": []map[string]any{
					{"id": "common-api", "use_cases": []string{"items", "finish"}},
					{"id": "tasks", "use_cases": []string{"progress", "collections"}},
				},
			},
		})
	case "evident_output.explain":
		rule, _ := args["rule_id"].(string)
		writeRPC(id, map[string]any{
			"content":           []map[string]any{{"type": "text", "text": "rule " + rule}},
			"structuredContent": map[string]any{"rule_id": rule, "rationale": "see architecture §31 and Appendix C"},
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
	case "evident_output.preview", "evident_output.get_guidance":
		writeRPC(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": name + " partial"}},
			"isError": false,
		})
	default:
		writeRPC(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": "unknown tool"}},
			"isError": true,
		})
	}
}
