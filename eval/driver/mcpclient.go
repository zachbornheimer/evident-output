package driver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

const (
	mcpProtocolVersion = "2025-06-18"
	mcpClientName      = "evident-output-eval"
	mcpMaxLineBytes    = 16 << 20
)

// ToolResult is what an MCP tool call returned.
type ToolResult struct {
	Text       string
	Structured json.RawMessage
	IsError    bool
}

// Render is the single string a model sees for a tool result: the text plus
// the structured payload when there is one.
func (r ToolResult) Render() string {
	if len(r.Structured) == 0 {
		return r.Text
	}
	return r.Text + "\n" + string(r.Structured)
}

// ToolServer is the MCP boundary: list tools, call a tool, close.
type ToolServer interface {
	Tools(ctx context.Context) ([]ToolDef, error)
	Call(ctx context.Context, name string, args json.RawMessage) (ToolResult, error)
	Close() error
}

// StdioMCP speaks newline-delimited JSON-RPC to an MCP server.
type StdioMCP struct {
	in     *bufio.Reader
	out    io.Writer
	closer func() error
	nextID int
}

// NewStdioMCP wraps an established stdio connection and performs the
// initialize handshake. closeFn releases the connection.
func NewStdioMCP(ctx context.Context, in io.Reader, out io.Writer, closeFn func() error) (*StdioMCP, error) {
	reader := bufio.NewReaderSize(in, mcpMaxLineBytes)
	server := &StdioMCP{in: reader, out: out, closer: closeFn}
	params := map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": mcpClientName, "version": "0"},
	}
	if _, err := server.request(ctx, "initialize", params); err != nil {
		return nil, fmt.Errorf("initialize MCP session: %w", err)
	}
	if err := server.write(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); err != nil {
		return nil, fmt.Errorf("send initialized notification: %w", err)
	}
	return server, nil
}

// SpawnMCP starts binary as a child process and connects to its stdio.
func SpawnMCP(ctx context.Context, binary string) (*StdioMCP, error) {
	proc := exec.CommandContext(ctx, binary)
	stdin, err := proc.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdin of %s: %w", binary, err)
	}
	stdout, err := proc.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdout of %s: %w", binary, err)
	}
	if err := proc.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", binary, err)
	}
	closeFn := func() error {
		_ = stdin.Close()
		_ = proc.Process.Kill()
		_ = proc.Wait()
		return nil
	}
	server, err := NewStdioMCP(ctx, stdout, stdin, closeFn)
	if err != nil {
		_ = closeFn()
		return nil, fmt.Errorf("connect to %s: %w", binary, err)
	}
	return server, nil
}

// Tools lists the server's tools.
func (s *StdioMCP) Tools(ctx context.Context) ([]ToolDef, error) {
	raw, err := s.request(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("list MCP tools: %w", err)
	}
	var result struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse MCP tools/list result: %w", err)
	}
	defs := make([]ToolDef, 0, len(result.Tools))
	for _, tool := range result.Tools {
		defs = append(defs, ToolDef{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema})
	}
	return defs, nil
}

// Call invokes one tool.
func (s *StdioMCP) Call(ctx context.Context, name string, args json.RawMessage) (ToolResult, error) {
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	raw, err := s.request(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return ToolResult{}, fmt.Errorf("call MCP tool %s: %w", name, err)
	}
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Structured json.RawMessage `json:"structuredContent"`
		IsError    bool            `json:"isError"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return ToolResult{}, fmt.Errorf("parse MCP tools/call result of %s: %w", name, err)
	}
	var text []string
	for _, part := range result.Content {
		text = append(text, part.Text)
	}
	return ToolResult{Text: strings.Join(text, "\n"), Structured: result.Structured, IsError: result.IsError}, nil
}

// Close releases the connection.
func (s *StdioMCP) Close() error {
	if s.closer == nil {
		return nil
	}
	return s.closer()
}

func (s *StdioMCP) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("send %s: %w", method, err)
	}
	s.nextID++
	id := s.nextID
	if err := s.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, fmt.Errorf("send %s: %w", method, err)
	}
	for {
		line, err := s.in.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("read reply to %s: %w", method, err)
		}
		var reply struct {
			ID     *int            `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(line, &reply); err != nil {
			return nil, fmt.Errorf("parse reply to %s: %w", method, err)
		}
		if reply.ID == nil || *reply.ID != id {
			continue
		}
		if reply.Error != nil {
			return nil, fmt.Errorf("%s failed with code %d: %s", method, reply.Error.Code, reply.Error.Message)
		}
		return reply.Result, nil
	}
}

func (s *StdioMCP) write(message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	if _, err := s.out.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	return nil
}
