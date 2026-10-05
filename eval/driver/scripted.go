package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrScriptExhausted means the loop asked for more turns than were recorded.
var ErrScriptExhausted = errors.New("scripted model has no more turns")

// ScriptedTurn is one recorded model reply and the usage it reports.
type ScriptedTurn struct {
	Response Response
	Usage    Usage
}

// ScriptedModel replays recorded turns in order and keeps every request it
// received so tests can assert on what the model was shown.
type ScriptedModel struct {
	Turns    []ScriptedTurn
	Requests []Request
}

// Complete returns the next recorded turn.
func (m *ScriptedModel) Complete(_ context.Context, req Request) (Response, Usage, error) {
	m.Requests = append(m.Requests, req)
	index := len(m.Requests) - 1
	if index >= len(m.Turns) {
		return Response{}, Usage{}, fmt.Errorf("scripted turn %d of %d: %w", index+1, len(m.Turns), ErrScriptExhausted)
	}
	turn := m.Turns[index]
	return turn.Response, turn.Usage, nil
}

// ToolCallTurn records a model reply that calls one tool.
func ToolCallTurn(id, tool string, input any, usage Usage) ScriptedTurn {
	raw, err := json.Marshal(input)
	if err != nil {
		panic(fmt.Sprintf("marshal scripted input for %s: %v", tool, err))
	}
	block := Block{Kind: BlockToolUse, ToolUseID: id, ToolName: tool, Input: raw}
	return ScriptedTurn{Response: Response{Blocks: []Block{block}, Stop: StopToolUse}, Usage: usage}
}

// TextTurn records a model reply that only talks and ends the turn.
func TextTurn(text string, usage Usage) ScriptedTurn {
	block := Block{Kind: BlockText, Text: text}
	return ScriptedTurn{Response: Response{Blocks: []Block{block}, Stop: StopEndTurn}, Usage: usage}
}
