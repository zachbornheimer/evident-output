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
	"time"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/catalog"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/rules"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/buildversion"
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
// installedVersionUnset marks an unstamped build; resolvedVersion never
// prints it, falling back to buildversion.PublishedRelease instead.
const installedVersionUnset = buildversion.Unset

var Version = installedVersionUnset

// resolvedVersion is what --version prints, in priority order: the
// ldflags-stamped Version when set (mise's build task, install.go's
// `go install -C <dir>` for a local/replace pin); else the module version
// Go itself embeds for `go install pkg@vX.Y.Z` (debug.BuildInfo.Main.Version
// — no ldflags needed, this is how `go version -m` reports any installed
// Go tool's pin); else the VCS revision Go auto-embeds into local
// go run/go build binaries built inside a git checkout (buildvcs=auto,
// default since Go 1.18) appended to buildversion.PublishedRelease as build metadata; else
// bare buildversion.PublishedRelease. A stale-host check ("is this binary older than
// the pin?") never has to trust the literal string "dev" (AGENTS.md
// "Dialect the MCP enforces" / evo-dialect-axes-report.md axis 4/12: a
// fresh --version printing "dev" cannot tell stale from fresh).
func resolvedVersion() string {
	return buildversion.Resolve(Version, buildversion.PublishedRelease, debug.ReadBuildInfo)
}

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
	// maxFrameBytes bounds one JSON-RPC message, in either framing.
	maxFrameBytes = 8 << 20 // 8 MiB
)

var toolNameRE = regexp.MustCompile(`^[a-z][a-z0-9_.]{0,63}$`)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Printf("evident-output-mcp %s\n", resolvedVersion())
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
			"resources": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "evident-output-mcp",
			"version": resolvedVersion(),
		},
		// Optional human hint (allowed on InitializeResult).
		"instructions": serverInstructions,
	})
}

// truncateForLog reports only length metadata — never payload bytes (may hold secrets/source).
func truncateForLog(b []byte, n int) string {
	return fmt.Sprintf("<%d bytes>", len(b))
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
