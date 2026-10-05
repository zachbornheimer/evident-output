package driver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// DefaultMaxRetries bounds the SDK's retry of 408, 409, 429 and 5xx replies;
// the SDK backs off exponentially and honors retry-after headers.
const DefaultMaxRetries = 4

// AnthropicModel is the real Claude Messages API client. Building one is the
// only way this module can spend money.
type AnthropicModel struct {
	client anthropic.Client
}

// NewAnthropicModel builds a client. Extra options (a test base URL, say)
// apply after the defaults.
func NewAnthropicModel(apiKey string, opts ...option.RequestOption) *AnthropicModel {
	all := append([]option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithMaxRetries(DefaultMaxRetries),
	}, opts...)
	return &AnthropicModel{client: anthropic.NewClient(all...)}
}

// Complete sends one Messages API request.
func (m *AnthropicModel) Complete(ctx context.Context, req Request) (Response, Usage, error) {
	params, err := BuildParams(req)
	if err != nil {
		return Response{}, Usage{}, fmt.Errorf("build request for model %s: %w", req.Model, err)
	}
	msg, err := m.client.Messages.New(ctx, params)
	if err != nil {
		return Response{}, Usage{}, fmt.Errorf("call messages API for model %s: %w", req.Model, err)
	}
	return responseFromMessage(msg), usageFromMessage(msg.Usage), nil
}

// BuildParams maps a Request onto Messages API parameters, placing a prompt
// cache breakpoint on the last system block and the last tool definition so
// the corpus and tool list are read from cache on every later turn.
func BuildParams(req Request) (anthropic.MessageNewParams, error) {
	messages, err := messageParams(req.Messages)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}
	tools, err := toolParams(req.Tools)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: req.MaxTokens,
		Messages:  messages,
		Tools:     tools,
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{
			Text:         req.System,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}}
	}
	return params, nil
}

func toolParams(defs []ToolDef) ([]anthropic.ToolUnionParam, error) {
	tools := make([]anthropic.ToolUnionParam, 0, len(defs))
	for i, def := range defs {
		var schema struct {
			Properties any      `json:"properties"`
			Required   []string `json:"required"`
		}
		if err := json.Unmarshal(def.InputSchema, &schema); err != nil {
			return nil, fmt.Errorf("parse input schema of tool %s: %w", def.Name, err)
		}
		tool := anthropic.ToolParam{
			Name:        def.Name,
			Description: anthropic.String(def.Description),
			InputSchema: anthropic.ToolInputSchemaParam{Properties: schema.Properties, Required: schema.Required},
		}
		if i == len(defs)-1 {
			tool.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		tools = append(tools, anthropic.ToolUnionParam{OfTool: &tool})
	}
	return tools, nil
}

func messageParams(messages []Message) ([]anthropic.MessageParam, error) {
	out := make([]anthropic.MessageParam, 0, len(messages))
	for _, message := range messages {
		blocks, err := blockParams(message.Blocks)
		if err != nil {
			return nil, err
		}
		if message.Role == RoleAssistant {
			out = append(out, anthropic.NewAssistantMessage(blocks...))
		} else {
			out = append(out, anthropic.NewUserMessage(blocks...))
		}
	}
	return out, nil
}

func blockParams(blocks []Block) ([]anthropic.ContentBlockParamUnion, error) {
	out := make([]anthropic.ContentBlockParamUnion, 0, len(blocks))
	for _, block := range blocks {
		switch block.Kind {
		case BlockText:
			out = append(out, anthropic.NewTextBlock(block.Text))
		case BlockToolUse:
			out = append(out, anthropic.NewToolUseBlock(block.ToolUseID, block.Input, block.ToolName))
		case BlockToolResult:
			out = append(out, anthropic.NewToolResultBlock(block.ToolUseID, block.Text, block.IsError))
		default:
			return nil, fmt.Errorf("convert message block of unknown kind %q", block.Kind)
		}
	}
	return out, nil
}

func responseFromMessage(msg *anthropic.Message) Response {
	resp := Response{Stop: StopReason(msg.StopReason)}
	for _, content := range msg.Content {
		switch content.Type {
		case string(BlockText):
			resp.Blocks = append(resp.Blocks, Block{Kind: BlockText, Text: content.Text})
		case string(BlockToolUse):
			resp.Blocks = append(resp.Blocks, Block{
				Kind: BlockToolUse, ToolUseID: content.ID, ToolName: content.Name, Input: content.Input,
			})
		}
	}
	return resp
}

func usageFromMessage(usage anthropic.Usage) Usage {
	return Usage{
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheReadTokens:  usage.CacheReadInputTokens,
		CacheWriteTokens: usage.CacheCreationInputTokens,
	}
}
