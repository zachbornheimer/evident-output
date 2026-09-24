// Command evident-output-mcp is the stdio MCP server.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zachbornheimer/evident-output/internal/agent/catalog"
	"github.com/zachbornheimer/evident-output/internal/agent/rules"
)

// serverInstructions is the MCP `instructions` hint returned on initialize —
// modeled on the official Svelte MCP server's contract ("This is the
// official Svelte MCP server. It MUST be used whenever svelte development
// is involved. ... After you correct the component call this tool again to
// confirm all the issues are fixed."): a directive to use the server, not
// just a description of it, and an explicit instruction to loop the review
// tool until it reports zero findings.
const serverInstructions = "This is the official Evident Output MCP server. It MUST be used whenever CLI output or presentation code is written or changed in a repo that uses evident-output (or is adopting it). Call evident_output_list_sections / evident_output_get_documentation for authoritative docs, evident_output_adopt_plan to inventory non-evo output in an existing codebase, and evident_output_review before treating any CLI output change as done. When review reports update_needed, call evident_output_update then restart the MCP host before treating review as done. When migrating existing evo call sites, evident_output_review is the autofixer (same role as svelte-autofixer): pass the Go source or kind=directory, apply every suggestion, and call again until zero findings. After applying evident_output_review's suggested fixes, call evident_output_review again on the same source — repeat until it reports zero findings (recheck_required=false); only then is the change clean. Catalog checksum available via resource evident-output://meta/catalog-checksum."

// Version is injected at build time via -ldflags "-X main.Version=...".
// installedVersionUnset stays "dev" only when neither the build-time stamp
// nor the Go toolchain's own VCS stamp is available — see resolvedVersion.
const installedVersionUnset = "dev"

var Version = installedVersionUnset

// resolvedVersion is what --version prints, in priority order: the
// ldflags-stamped Version when set (mise's build task, install.go's
// `go install -C <dir>` for a local/replace pin); else the module version
// Go itself embeds for `go install pkg@vX.Y.Z` (debug.BuildInfo.Main.Version
// — no ldflags needed, this is how `go version -m` reports any installed
// Go tool's pin); else the VCS revision Go auto-embeds into local
// go run/go build binaries built inside a git checkout (buildvcs=auto,
// default since Go 1.18). A stale-host check ("is this binary older than
// the pin?") never has to trust the literal string "dev" (AGENTS.md
// "Dialect the MCP enforces" / evo-dialect-axes-report.md axis 4/12: a
// fresh --version printing "dev" cannot tell stale from fresh).
func resolvedVersion() string {
	if Version != installedVersionUnset {
		return Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Version
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var revision string
	var modified bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if revision == "" {
		return Version
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	stamp := "devel+" + revision
	if modified {
		stamp += "-dirty"
	}
	return stamp
}

// faultHook is an optional test-only injector for MCP-034 panic containment.
// Production always leaves this nil.
var faultHook func(toolName string)

// Supported protocol versions (MCP-041).
// Include the revision Grok and other recent hosts negotiate (2025-06-18).
var supportedProtocols = map[string]bool{
	"2024-11-05": true,
	"2025-03-26": true,
	"2025-06-18": true}

// latestProtocol is the highest revision in supportedProtocols. Per the MCP
// lifecycle spec, a server that does not recognize the client's requested
// protocolVersion MUST still respond successfully, offering a version it
// does support (never an error) — the client then decides whether to
// proceed. This is the version offered in that case.
const latestProtocol = "2025-06-18"

const (
	defaultToolDeadline = 30 * time.Second
	toolNameMaxLen      = 64
	// maxFrameBytes bounds one JSON-RPC message, in either framing.
	maxFrameBytes = 8 << 20 // 8 MiB
)

var toolNameRE = regexp.MustCompile(`^[a-z][a-z0-9_.]{0,63}$`)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Fprintf(os.Stderr, "evident-output-mcp %s\n", resolvedVersion())
		os.Exit(0)
	}
	if len(os.Args) > 1 {
		if code := runConfig(os.Args[1:]); code >= 0 {
			os.Exit(code)
		}
		if code := runUpdate(os.Args[1:]); code >= 0 {
			os.Exit(code)
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		maybeAutoUpdate(cwd, os.Args, osEnv{}, osFiles{}, execRunner{}, syscallExecer{}, liveIdentity())
	}
	// Log only to stderr (MCP stdio: stdout is protocol-only).
	fmt.Fprintf(os.Stderr, "evident-output-mcp %s starting (stdio)\n", resolvedVersion())
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
	var srv server
	r := bufio.NewReaderSize(in, 1024*1024)
	for {
		msg, mode, err := readMCPMessage(r)
		if errors.Is(err, errFrameTooLarge) {
			writeRPCError(nil, -32600, fmt.Sprintf("request exceeds %d MiB", maxFrameBytes>>20))
			continue
		}
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
		srv.handle(msg, mode)
	}
}

// server is one stdio session's lifecycle state.
type server struct {
	initialized bool
}

// handle answers one JSON-RPC message.
func (s *server) handle(msg []byte, mode framingMode) {
	var req map[string]any
	if err := json.Unmarshal(msg, &req); err != nil {
		fmt.Fprintf(os.Stderr, "parse error (%v): %q\n", mode, truncateForLog(msg, 120))
		writeRPCError(nil, -32700, "parse error")
		return
	}
	method, _ := req["method"].(string)
	id := req["id"]
	// MCP-001: reject out-of-lifecycle tool/resource calls before initialize.
	if !s.initialized && method != "initialize" && method != "ping" {
		if id != nil {
			writeRPCError(id, -32002, "server not initialized; call initialize first")
		}
		return
	}
	switch method {
	case "initialize":
		s.initialized = true
		handleInitialize(id, req)
	case "tools/list":
		writeRPC(id, map[string]any{"tools": toolList()})
	case "tools/call":
		safeToolCall(id, req)
	case "resources/list":
		writeRPC(id, map[string]any{
			"resources": []map[string]any{
				{"uri": "evident-output://guides/common-api", "name": "common-api", "mimeType": "text/plain"},
				{"uri": "evident-output://rules/API-006", "name": "API-006", "mimeType": "application/json"},
				{"uri": "evident-output://meta/catalog-checksum", "name": "catalog-checksum", "mimeType": "text/plain"}}})
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

// handleInitialize answers initialize with the negotiated protocol.
func handleInitialize(id any, req map[string]any) {
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
	// serverInfo: only name/version/title per lifecycle schema — no custom fields
	// (strict hosts reject unknown InitializeResult properties).
	writeRPC(id, map[string]any{
		"protocolVersion": negotiated,
		"capabilities": map[string]any{
			// Empty objects advertise the capability groups we implement.
			"tools":     map[string]any{},
			"resources": map[string]any{}},
		"serverInfo": map[string]any{
			"name":    "evident-output-mcp",
			"version": resolvedVersion()},
		// Optional human hint (allowed on InitializeResult).
		"instructions": serverInstructions})
}

// truncateForLog reports only length metadata — never payload bytes (may hold secrets/source).
func truncateForLog(b []byte, n int) string {
	return fmt.Sprintf("<%d bytes>", len(b))
}

func toolList() []map[string]any {
	tools := []map[string]any{
		{"name": "evident_output_list_sections", "description": "List the full docs corpus servable via evident_output_get_documentation (reference, development, MCP wiring, adoption ladder, per-concept guides)", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":       map[string]any{"type": "string"},
				"deadline_ms": map[string]any{"type": "integer"}}}},
		{"name": "evident_output_get_documentation", "description": "Retrieve one or more documentation sections by id (see evident_output_list_sections)", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ids":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"deadline_ms": map[string]any{"type": "integer"}}}},
		{"name": "evident_output_adopt_plan", "description": "Inventory non-evo CLI output (fmt.Print*/log.*/os.Stdout/spinner libs) under a directory and return a paged migration plan keyed to the adoption ladder", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"directory":   map[string]any{"type": "string"},
				"cursor":      map[string]any{"type": "string"},
				"limit":       map[string]any{"type": "integer"},
				"deadline_ms": map[string]any{"type": "integer"}},
			"required": []string{"directory"}}},
		{"name": "evident_output_review", "description": "Review Go source, a local directory, multi-file package, transcript, or structured JSON for evo misuse. MUST be used when migrating existing evo call sites (the autofixer, same role as svelte-autofixer): pass Go source or kind=directory, apply every suggestion, call again until zero findings", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source":          map[string]any{"type": "string"},
				"file":            map[string]any{"type": "string"},
				"directory":       map[string]any{"type": "string"},
				"kind":            map[string]any{"type": "string"},
				"files":           map[string]any{"type": "object"},
				"desired_version": map[string]any{"type": "string"},
				"deadline_ms":     map[string]any{"type": "integer"}}}},
		{"name": "evident_output_conformance", "description": "Answer whether a repo's Evo usage is current: what is stale, unsafe, mechanically upgradable, has explicit vs opaque provenance, or needs developer intent. Same inputs as evident_output_review (source/file or kind=directory); returns {target_version, findings[{rule,severity,file,line,summary,migration}]}", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source":         map[string]any{"type": "string"},
				"file":           map[string]any{"type": "string"},
				"directory":      map[string]any{"type": "string"},
				"kind":           map[string]any{"type": "string"},
				"target_version": map[string]any{"type": "string"},
				"deadline_ms":    map[string]any{"type": "integer"}}}},
		{"name": "evident_output_update", "description": "Install a matching evident-output-mcp binary for a go.mod pin (--directory) or a release tag (--version), then symlink into ~/.local/bin. Requires version XOR directory. After it returns, restart the MCP host.", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"version":     map[string]any{"type": "string"},
				"directory":   map[string]any{"type": "string"},
				"deadline_ms": map[string]any{"type": "integer"}}}},
		{"name": "evident_output_preview", "description": "Preview plain profiles for a declarative scene", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"subject":     map[string]any{"type": "string"},
				"item":        map[string]any{"type": "string"},
				"state":       map[string]any{"type": "string"},
				"debug":       map[string]any{"type": "string"},
				"deadline_ms": map[string]any{"type": "integer"}}}},
		{"name": "evident_output_explain", "description": "Explain a stable rule ID", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"rule_id":     map[string]any{"type": "string"},
				"deadline_ms": map[string]any{"type": "integer"}}}}}
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
					"error":  fmt.Sprint(r)}})
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
			"error":  msg}}
}

// toolArgAllowlist is the argument-name registry validateArgs enforces —
// factored out so TestToolRegistryMatchesToolList can hold it against
// toolList() without duplicating the literal.
func toolArgAllowlist() map[string]map[string]bool {
	return map[string]map[string]bool{
		"evident_output_list_guides": {"use_case": true, "max_tokens": true, "deadline_ms": true},
		"evident_output_get_guidance": {
			"ids": true, "max_tokens": true, "deadline_ms": true},
		"evident_output_review": {
			"source": true, "file": true, "directory": true, "kind": true, "files": true, "desired_version": true, "deadline_ms": true},
		"evident_output_conformance": {
			"source": true, "file": true, "directory": true, "kind": true, "desired_version": true, "target_version": true, "deadline_ms": true},
		"evident_output_update": {"version": true, "directory": true, "deadline_ms": true},
		"evident_output_preview": {
			"subject": true, "item": true, "state": true, "debug": true, "deadline_ms": true},
		"evident_output_explain":       {"rule_id": true, "deadline_ms": true},
		"evident_output_list_sections": {"query": true, "deadline_ms": true},
		"evident_output_get_documentation": {
			"ids": true, "deadline_ms": true},
		"evident_output_adopt_plan": {"directory": true, "cursor": true, "limit": true, "deadline_ms": true}}
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
				"uri": uri, "mimeType": "text/plain", "text": catalog.Checksum()}}})
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
			"contents": []map[string]any{{"uri": uri, "mimeType": "text/plain", "text": found[0].Body}}})
	case len(uri) > len("evident-output://rules/") && uri[:len("evident-output://rules/")] == "evident-output://rules/":
		rid := uri[len("evident-output://rules/"):]
		if containsTraversal(rid) {
			writeRPCError(id, -32002, "resource not found")
			return
		}
		if r, ok := rules.Explain(rid); ok {
			b, _ := json.Marshal(r)
			writeRPC(id, map[string]any{
				"contents": []map[string]any{{"uri": uri, "mimeType": "application/json", "text": string(b)}}})
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
		Error:   map[string]any{"code": code, "message": message}})
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
