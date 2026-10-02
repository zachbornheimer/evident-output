// Package driver runs a model through a pit-of-success sample: it sees only
// the evo docs and the MCP tools, writes a Go program for a task, and submits
// it for grading. Nothing here touches the network unless an AnthropicModel is
// constructed; every other Model is local.
package driver

import (
	"context"
	"encoding/json"
)

// Role is who authored a Message.
type Role string

// Message roles.
const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// BlockKind is the shape of one content Block.
type BlockKind string

// Block kinds.
const (
	BlockText       BlockKind = "text"
	BlockToolUse    BlockKind = "tool_use"
	BlockToolResult BlockKind = "tool_result"
)

// StopReason is why a model turn ended.
type StopReason string

// Stop reasons the loop distinguishes.
const (
	StopToolUse StopReason = "tool_use"
	StopEndTurn StopReason = "end_turn"
)

// Block is one piece of message content. Text carries the text of a text
// block or the content of a tool result.
type Block struct {
	Kind      BlockKind       `json:"kind"`
	Text      string          `json:"text,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	ToolName  string          `json:"tool_name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

// Message is one conversation turn.
type Message struct {
	Role   Role    `json:"role"`
	Blocks []Block `json:"blocks"`
}

// ToolDef describes one tool the model may call.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// Request is one model call.
type Request struct {
	Model     string
	MaxTokens int64
	System    string
	Tools     []ToolDef
	Messages  []Message
}

// Response is the model's reply to one Request.
type Response struct {
	Blocks []Block
	Stop   StopReason
}

// ToolUses are the tool calls in the response, in order.
func (r Response) ToolUses() []Block {
	var uses []Block
	for _, block := range r.Blocks {
		if block.Kind == BlockToolUse {
			uses = append(uses, block)
		}
	}
	return uses
}

// Usage is the billed token counts of one Request.
type Usage struct {
	InputTokens      int64 `json:"input"`
	OutputTokens     int64 `json:"output"`
	CacheReadTokens  int64 `json:"cache_read"`
	CacheWriteTokens int64 `json:"cache_write"`
}

// Add returns the sum of two usages.
func (u Usage) Add(other Usage) Usage {
	return Usage{
		InputTokens:      u.InputTokens + other.InputTokens,
		OutputTokens:     u.OutputTokens + other.OutputTokens,
		CacheReadTokens:  u.CacheReadTokens + other.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens + other.CacheWriteTokens,
	}
}

// Model answers one request. It is the only seam through which a sample can
// spend money.
type Model interface {
	Complete(ctx context.Context, req Request) (Response, Usage, error)
}
