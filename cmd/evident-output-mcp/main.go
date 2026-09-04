// Command evident-output-mcp is the stdio MCP server.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/agent/adopt"
	"github.com/zachbornheimer/evident-output/internal/agent/catalog"
	"github.com/zachbornheimer/evident-output/internal/agent/preview"
	"github.com/zachbornheimer/evident-output/internal/agent/review"
	"github.com/zachbornheimer/evident-output/internal/agent/rules"
	"github.com/zachbornheimer/evident-output/internal/agent/sections"
)

// serverInstructions is the MCP `instructions` hint returned on initialize —
// modeled on the official Svelte MCP server's contract ("This is the
// official Svelte MCP server. It MUST be used whenever svelte development
// is involved. ... After you correct the component call this tool again to
// confirm all the issues are fixed."): a directive to use the server, not
// just a description of it, and an explicit instruction to loop the review
// tool until it reports zero findings.
const serverInstructions = "This is the official Evident Output MCP server. It MUST be used whenever CLI output or presentation code is written or changed in a repo that uses evident-output (or is adopting it). Call evident_output_list_sections / evident_output_get_documentation for authoritative docs, evident_output_adopt_plan to inventory non-evo output in an existing codebase, and evident_output_review before treating any CLI output change as done. After applying evident_output_review's suggested fixes, call evident_output_review again on the same source — repeat until it reports zero findings (recheck_required=false); only then is the change clean. Catalog checksum available via resource evident-output://meta/catalog-checksum."

// Version is injected at build time.
var Version = "dev"

// faultHook is an optional test-only injector for MCP-034 panic containment.
// Production always leaves this nil.
var faultHook func(toolName string)

// Supported protocol versions (MCP-041).
// Include the revision Grok and other recent hosts negotiate (2025-06-18).
var supportedProtocols = map[string]bool{
	"2024-11-05": true,
	"2025-03-26": true,
	"2025-06-18": true,
}

// latestProtocol is the highest revision in supportedProtocols. Per the MCP
// lifecycle spec, a server that does not recognize the client's requested
// protocolVersion MUST still respond successfully, offering a version it
// does support (never an error) — the client then decides whether to
// proceed. This is the version offered in that case.
const latestProtocol = "2025-06-18"

const (
	defaultToolDeadline = 30 * time.Second
	toolNameMaxLen      = 64
	// maxNDJSONFrameBytes bounds a single newline-delimited JSON-RPC message.
	maxNDJSONFrameBytes = 8 << 20 // 8 MiB, same as Content-Length path
)

var toolNameRE = regexp.MustCompile(`^[a-z][a-z0-9_.]{0,63}$`)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Fprintf(os.Stderr, "evident-output-mcp %s\n", Version)
		os.Exit(0)
	}
	if len(os.Args) > 1 {
		if code := runConfig(os.Args[1:]); code >= 0 {
			os.Exit(code)
		}
	}
	// Log only to stderr (MCP stdio: stdout is protocol-only).
	fmt.Fprintf(os.Stderr, "evident-output-mcp %s starting (stdio)\n", Version)
	runStdioServer(os.Stdin, os.Stdout)
}

// framingMode tracks how the current client frames messages on stdio.
// Spec stdio is newline-delimited JSON (2025-06-18 transports). Some hosts
// (and older SDK builds) still send LSP-style Content-Length frames; we accept
// both and reply in the mode of the last request.
type framingMode int

const (
	frameNDJSON framingMode = iota
	frameContentLength
)

// outMu serializes framed writes to stdout.
var (
	outMu   sync.Mutex
	outMode = frameNDJSON
	// outW is the protocol writer (defaults to os.Stdout; tests may replace).
	outW io.Writer = os.Stdout
)

func runStdioServer(in io.Reader, out io.Writer) {
	outMu.Lock()
	outW = out
	outMode = frameNDJSON
	outMu.Unlock()
	initialized := false
	r := bufio.NewReaderSize(in, 1024*1024)
	for {
		msg, mode, err := readMCPMessage(r)
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(os.Stderr, "stdin: %v\n", err)
				os.Exit(1)
			}
			return
		}
		if len(msg) == 0 {
			continue
		}
		outMu.Lock()
		outMode = mode
		outMu.Unlock()

		var req map[string]any
		if err := json.Unmarshal(msg, &req); err != nil {
			fmt.Fprintf(os.Stderr, "parse error (%v): %q\n", mode, truncateForLog(msg, 120))
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
				if supportedProtocols[clientProto] {
					negotiated = clientProto
				} else {
					// Unknown/newer client version: per spec, negotiate down to
					// our latest supported version rather than erroring — the
					// client decides whether our version works for it.
					negotiated = latestProtocol
				}
			}
			initialized = true
			// serverInfo: only name/version/title per lifecycle schema — no custom fields
			// (strict hosts reject unknown InitializeResult properties).
			writeRPC(id, map[string]any{
				"protocolVersion": negotiated,
				"capabilities": map[string]any{
					// Empty objects advertise the capability groups we implement.
					"tools":     map[string]any{},
					"resources": map[string]any{},
				},
				"serverInfo": map[string]any{
					"name":    "evident-output-mcp",
					"version": Version,
				},
				// Optional human hint (allowed on InitializeResult).
				"instructions": serverInstructions,
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
			// notifications/initialized has no id and no response.
			// ping may carry an id (utilities/ping).
			if id != nil {
				writeRPC(id, map[string]any{})
			}
		default:
			if id != nil {
				writeRPCError(id, -32601, "method not found: "+method)
			}
		}
	}
}

// readMCPMessage reads one JSON-RPC message from r.
// Supports NDJSON (spec) and LSP-style Content-Length frames (some clients).
func readMCPMessage(r *bufio.Reader) ([]byte, framingMode, error) {
	// Peek for Content-Length without consuming a bare JSON line.
	for {
		// Skip leading CR/LF.
		b, err := r.ReadByte()
		if err != nil {
			return nil, frameNDJSON, err
		}
		if b == '\n' || b == '\r' {
			continue
		}
		if err := r.UnreadByte(); err != nil {
			return nil, frameNDJSON, err
		}
		break
	}

	peek, err := r.Peek(1)
	if err != nil {
		return nil, frameNDJSON, err
	}
	// Content-Length header (case-insensitive) — used by some MCP client SDKs.
	if peek[0] == 'C' || peek[0] == 'c' {
		headerLine, err := r.ReadString('\n')
		if err != nil {
			return nil, frameContentLength, err
		}
		headerLine = strings.TrimRight(headerLine, "\r\n")
		if !strings.HasPrefix(strings.ToLower(headerLine), "content-length:") {
			// Not a content-length header; treat as broken NDJSON starting with C.
			return []byte(headerLine), frameNDJSON, nil
		}
		nStr := strings.TrimSpace(headerLine[len("Content-Length:"):])
		// header may be "content-length:" with different case
		if i := strings.Index(strings.ToLower(headerLine), ":"); i >= 0 {
			nStr = strings.TrimSpace(headerLine[i+1:])
		}
		n, err := strconv.Atoi(nStr)
		if err != nil || n < 0 || n > 8<<20 {
			return nil, frameContentLength, fmt.Errorf("invalid Content-Length %q", nStr)
		}
		// Consume optional additional headers until blank line.
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return nil, frameContentLength, err
			}
			if line == "\n" || line == "\r\n" {
				break
			}
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(r, body); err != nil {
			return nil, frameContentLength, err
		}
		return body, frameContentLength, nil
	}

	// NDJSON: one JSON object per line, hard-capped.
	var buf bytes.Buffer
	for {
		b, err := r.ReadByte()
		if err != nil {
			if buf.Len() == 0 {
				return nil, frameNDJSON, err
			}
			// Incomplete final frame without newline.
			break
		}
		if b == '\n' {
			break
		}
		if buf.Len() >= maxNDJSONFrameBytes {
			// Drain until newline or EOF so the next message can resync.
			for {
				bb, e2 := r.ReadByte()
				if e2 != nil || bb == '\n' {
					break
				}
			}
			return nil, frameNDJSON, fmt.Errorf("ndjson frame exceeds %d bytes", maxNDJSONFrameBytes)
		}
		buf.WriteByte(b)
	}
	line := bytes.TrimRight(buf.Bytes(), "\r")
	return line, frameNDJSON, nil
}

// truncateForLog reports only length metadata — never payload bytes (may hold secrets/source).
func truncateForLog(b []byte, n int) string {
	return fmt.Sprintf("<%d bytes>", len(b))
}

func toolList() []map[string]any {
	tools := []map[string]any{
		{"name": "evident_output_list_guides", "description": "List guidance catalog entries", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"use_case":    map[string]any{"type": "string"},
				"max_tokens":  map[string]any{"type": "integer"},
				"deadline_ms": map[string]any{"type": "integer"},
			},
		}},
		{"name": "evident_output_get_guidance", "description": "Retrieve guidance sections by id", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ids":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"max_tokens":  map[string]any{"type": "integer"},
				"deadline_ms": map[string]any{"type": "integer"},
			},
		}},
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
		{"name": "evident_output_adopt_plan", "description": "Inventory non-evo CLI output (fmt.Print*/log.*/os.Stdout/spinner libs) under a directory and return a migration plan keyed to the adoption ladder", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"directory":   map[string]any{"type": "string"},
				"deadline_ms": map[string]any{"type": "integer"},
			},
			"required": []string{"directory"},
		}},
		{"name": "evident_output_review", "description": "Review Go source, multi-file package, transcript, or structured JSON for evo misuse", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source":      map[string]any{"type": "string"},
				"file":        map[string]any{"type": "string"},
				"kind":        map[string]any{"type": "string"},
				"files":       map[string]any{"type": "object"},
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
	if strings.HasPrefix(name, "evident_output.") {
		return "evident_output_" + strings.TrimPrefix(name, "evident_output.")
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

	switch name {
	case "evident_output_list_guides":
		useCase, _ := args["use_case"].(string)
		guides := catalog.Filter(useCase)
		maxTok := intFromArgs(args, "max_tokens")
		truncated := false
		if maxTok > 0 {
			guides, truncated = catalog.ApplyTokenBudget(guides, maxTok)
		}
		if cancelled.Load() {
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
	case "evident_output_get_guidance":
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
		if cancelled.Load() {
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
	case "evident_output_explain":
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
	case "evident_output_list_sections":
		query, _ := args["query"].(string)
		list := sections.Filter(query)
		if cancelled.Load() {
			writeRPC(id, toolError("deadline exceeded"))
			return
		}
		writeRPC(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("%d sections", len(list))}},
			"structuredContent": map[string]any{
				"schema":   "evident_output.sections.v1",
				"sections": summarizeSections(list),
			},
		})
	case "evident_output_get_documentation":
		var ids []string
		if raw, ok := args["ids"].([]any); ok {
			for _, v := range raw {
				if s, ok := v.(string); ok {
					ids = append(ids, s)
				}
			}
		}
		var found []sections.Section
		var missing []string
		for _, sid := range ids {
			if s, ok := sections.Get(sid); ok {
				found = append(found, s)
			} else {
				missing = append(missing, sid)
			}
		}
		if cancelled.Load() {
			writeRPC(id, toolError("deadline exceeded"))
			return
		}
		writeRPC(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("found=%d missing=%d", len(found), len(missing))}},
			"structuredContent": map[string]any{
				"schema":   "evident_output.documentation.v1",
				"sections": found,
				"missing":  missing,
			},
		})
	case "evident_output_adopt_plan":
		directory, _ := args["directory"].(string)
		if directory == "" {
			writeRPC(id, toolError("directory is required"))
			return
		}
		if isRemotePath(directory) {
			writeRPC(id, toolError("remote path unsupported; pass a local directory (MCP-036)"))
			return
		}
		plan, err := adopt.Inventory(directory)
		if err != nil {
			writeRPC(id, toolError("adopt_plan: "+err.Error()))
			return
		}
		if cancelled.Load() {
			writeRPC(id, toolError("deadline exceeded"))
			return
		}
		writeRPC(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("%d findings across %d ladder rungs", len(plan.Findings), len(plan.RungsTouched))}},
			"structuredContent": map[string]any{
				"schema": "evident_output_adopt_plan.v1",
				"plan":   plan,
			},
		})
	case "evident_output_review":
		src, _ := args["source"].(string)
		file, _ := args["file"].(string)
		kind, _ := args["kind"].(string)
		if file == "" {
			file = "input.go"
		}
		// MCP-036: remote paths unsupported — accept inlined content, or a
		// readable local absolute path.
		if isRemotePath(file) {
			writeRPC(id, toolError("remote path unsupported; pass source content only (MCP-036)"))
			return
		}
		if src == "" && filepath.IsAbs(file) {
			read, err := os.ReadFile(file)
			if err != nil {
				writeRPC(id, toolError(fmt.Sprintf("cannot read %s: %s", file, err)))
				return
			}
			src = string(read)
		}
		var res review.Result
		switch kind {
		case "transcript":
			res = review.Transcript(file, src)
		case "json", "structured":
			res = review.StructuredDocument(file, []byte(src))
		case "package":
			// MCP-017: multi-file map via JSON object in source or single pair.
			// Each map value may be inline source text or a readable local
			// absolute path — resolved the same way as the single `file` form.
			files := map[string]string{file: src}
			if raw, ok := args["files"].(map[string]any); ok {
				files = map[string]string{}
				for k, v := range raw {
					s, ok := v.(string)
					if !ok {
						continue
					}
					if isRemotePath(s) {
						writeRPC(id, toolError("remote path unsupported; pass source content only (MCP-036)"))
						return
					}
					if filepath.IsAbs(s) {
						read, err := os.ReadFile(s)
						if err != nil {
							writeRPC(id, toolError(fmt.Sprintf("cannot read %s: %s", s, err)))
							return
						}
						s = string(read)
					}
					files[k] = s
				}
			}
			if allFileContentEmpty(files) {
				writeRPC(id, toolError("empty source after decode: check files map shape"))
				return
			}
			res = review.GoPackage(files)
		default:
			if src == "" {
				writeRPC(id, toolError("no source to review: pass `source` content or an absolute `file` path that exists"))
				return
			}
			res = review.GoSource(file, src)
		}
		if cancelled.Load() {
			writeRPC(id, toolError("deadline exceeded"))
			return
		}
		nextAction := reviewNextAction(res)
		text := fmt.Sprintf("findings=%d recheck=%v partial=%v — %s", len(res.Findings), res.RecheckRequired, res.Partial, nextAction)
		writeRPC(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"structuredContent": map[string]any{
				"schema":           "evident_output_review.v1",
				"recheck_required": res.RecheckRequired,
				"partial":          res.Partial,
				"findings":         res.Findings,
				"next_action":      nextAction,
			},
		})
	case "evident_output_preview":
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
		out := evo.Init(evo.Config{Options: []evo.Option{evo.Title(subject), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DebugLevel(evo.LevelDebug)}})
		it := out.Task(item)
		switch state {
		case "blocked":
			it.Block("blocked for demo")
		case "failed":
			it.Fail("failed for demo")
		default:
			it.Done()
		}
		if dbg != "" {
			out.Debug(dbg)
		}
		_ = out.Finish()
		snap := out.Snapshot()
		profiles := preview.DefaultProfiles(snap)
		if cancelled.Load() {
			writeRPC(id, toolError("deadline exceeded"))
			return
		}
		writeRPC(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("%d profiles", len(profiles))}},
			"structuredContent": map[string]any{
				"schema":   "evident_output_preview.v1",
				"profiles": profiles,
				"plain":    buf.String(),
			},
		})
	default:
		writeRPC(id, toolError("unknown tool"))
	}
}

// allFileContentEmpty reports whether every entry in a package-kind `files`
// map decoded to no usable content — e.g. the map held non-string values, or
// resolved paths read as empty. This is the honest diagnosis for the "empty
// source" failure mode: a generic parser EOF error tells the caller nothing
// about which of these two shapes actually happened.
func allFileContentEmpty(files map[string]string) bool {
	for _, content := range files {
		if content != "" {
			return false
		}
	}
	return true
}

// reviewNextAction makes the review→fix→re-review loop self-driving: an
// agent that only reads this field (never the findings count itself) still
// knows whether to stop or keep going, matching the Svelte MCP's "call this
// tool again to confirm all the issues are fixed" instruction.
func reviewNextAction(res review.Result) string {
	if len(res.Findings) == 0 && !res.RecheckRequired {
		return "clean: 0 findings, no recheck needed"
	}
	return "re-run evident_output_review after applying the suggested fixes, until findings=0 and recheck_required=false"
}

// summarizeSections strips body text for the list view — evident_output_list_sections
// is a table of contents; evident_output_get_documentation returns the body.
func summarizeSections(list []sections.Section) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, s := range list {
		out = append(out, map[string]any{
			"id":       s.ID,
			"title":    s.Title,
			"source":   s.Source,
			"concepts": s.Concepts,
		})
	}
	return out
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
			"source": true, "file": true, "kind": true, "files": true, "deadline_ms": true,
		},
		"evident_output_preview": {
			"subject": true, "item": true, "state": true, "debug": true, "deadline_ms": true,
		},
		"evident_output_explain":       {"rule_id": true, "deadline_ms": true},
		"evident_output_list_sections": {"query": true, "deadline_ms": true},
		"evident_output_get_documentation": {
			"ids": true, "deadline_ms": true,
		},
		"evident_output_adopt_plan": {"directory": true, "deadline_ms": true},
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
	// Preserve field order clients often expect: jsonrpc, id, result.
	writeFramed(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      any    `json:"id"`
		Result  any    `json:"result"`
	}{JSONRPC: "2.0", ID: id, Result: result})
}

func writeRPCError(id any, code int, message string) {
	writeFramed(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      any    `json:"id"`
		Error   any    `json:"error"`
	}{
		JSONRPC: "2.0",
		ID:      id,
		Error:   map[string]any{"code": code, "message": message},
	})
}

func writeFramed(v any) {
	outMu.Lock()
	defer outMu.Unlock()
	data, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
		return
	}
	// Messages MUST NOT contain embedded newlines (stdio transport).
	switch outMode {
	case frameContentLength:
		if _, err := fmt.Fprintf(outW, "Content-Length: %d\r\n\r\n%s", len(data), data); err != nil {
			fmt.Fprintf(os.Stderr, "write: %v\n", err)
			return
		}
	default:
		if _, err := outW.Write(data); err != nil {
			fmt.Fprintf(os.Stderr, "write: %v\n", err)
			return
		}
		if _, err := outW.Write([]byte{'\n'}); err != nil {
			fmt.Fprintf(os.Stderr, "write: %v\n", err)
		}
	}
}
