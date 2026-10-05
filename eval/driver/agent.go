package driver

import (
	"context"
	"fmt"
)

// Loop limits.
const (
	MaxToolTurns     = 40
	DefaultMaxTokens = 8192
)

// Why a sample ended without a submission.
const (
	EndedTurnCap   = "tool turn cap reached"
	EndedNoSubmit  = "model ended its turn without submitting"
	EndedSubmitted = "submitted"
)

const taskInstructions = `Write the program as Go source files in package main. The domain package is importable as "evalsandbox/fixture". Use the documentation in the system prompt and the evident_output_* tools; build with go_build and call review until it is clean. Call submit with the final files when done.`

// Meter prices usage and enforces the spend cap.
type Meter struct {
	Price Price
	Guard *SpendGuard
}

// Record bills one usage and returns its cost. The error is
// ErrSpendCapReached once the cap is reached.
func (m Meter) Record(usage Usage) (float64, error) {
	cost := m.Price.Cost(usage)
	return cost, m.Guard.Add(cost)
}

// Sample is one task attempt's inputs.
type Sample struct {
	Model     Model
	ModelID   string
	MaxTokens int64
	System    string
	Prompt    string // task prompt plus fixture API summary
	Tools     Toolbox
	Meter     Meter
}

// SampleResult is everything observed in one attempt.
type SampleResult struct {
	Turns     int               `json:"turns"`
	ToolCalls int               `json:"tool_calls"`
	Files     map[string]string `json:"files,omitempty"`
	Submitted bool              `json:"submitted"`
	Ended     string            `json:"ended"`
	// CyclesToClean counts review calls up to and including the first clean
	// one; -1 when review never came back clean.
	CyclesToClean int     `json:"cycles_to_clean"`
	Usage         Usage   `json:"usage"`
	CostUSD       float64 `json:"cost_usd"`
}

// Run drives the tool-use loop until the model submits, ends its turn, or hits
// the turn cap. When the spend cap trips it returns the partial result with
// an error wrapping ErrSpendCapReached.
func (s Sample) Run(ctx context.Context) (SampleResult, error) {
	result := SampleResult{CyclesToClean: -1}
	defs, err := s.Tools.Definitions(ctx)
	if err != nil {
		return result, err
	}
	reviews := 0
	messages := []Message{{Role: RoleUser, Blocks: []Block{{Kind: BlockText, Text: s.Prompt + "\n\n" + taskInstructions}}}}
	for result.Turns < MaxToolTurns {
		if err := s.Meter.Guard.Check(); err != nil {
			return result, err
		}
		req := Request{Model: s.ModelID, MaxTokens: s.MaxTokens, System: s.System, Tools: defs, Messages: messages}
		resp, usage, err := s.Model.Complete(ctx, req)
		if err != nil {
			return result, fmt.Errorf("model turn %d: %w", result.Turns+1, err)
		}
		result.Turns++
		result.Usage = result.Usage.Add(usage)
		cost, spendErr := s.Meter.Record(usage)
		result.CostUSD += cost
		if spendErr != nil {
			return result, spendErr
		}
		messages = append(messages, Message{Role: RoleAssistant, Blocks: resp.Blocks})
		uses := resp.ToolUses()
		if len(uses) == 0 {
			result.Ended = EndedNoSubmit
			return result, nil
		}
		reply, err := s.answerToolCalls(ctx, uses, &result, &reviews)
		if err != nil {
			return result, err
		}
		if result.Submitted {
			return result, nil
		}
		messages = append(messages, reply)
	}
	result.Ended = EndedTurnCap
	return result, nil
}

// answerToolCalls dispatches one turn's tool calls and builds the reply
// message. A submit call ends the sample: result.Submitted is set and the
// reply is unused.
func (s Sample) answerToolCalls(ctx context.Context, uses []Block, result *SampleResult, reviews *int) (Message, error) {
	reply := Message{Role: RoleUser}
	for _, use := range uses {
		result.ToolCalls++
		outcome, err := s.Tools.Dispatch(ctx, use)
		if err != nil {
			return Message{}, fmt.Errorf("tool %s on turn %d: %w", use.ToolName, result.Turns, err)
		}
		if outcome.Submission != nil {
			result.Files, result.Submitted, result.Ended = outcome.Submission, true, EndedSubmitted
			return reply, nil
		}
		if outcome.ReviewClean != nil {
			*reviews++
			if *outcome.ReviewClean && result.CyclesToClean < 0 {
				result.CyclesToClean = *reviews
			}
		}
		reply.Blocks = append(reply.Blocks, Block{Kind: BlockToolResult, ToolUseID: use.ToolUseID, Text: outcome.Text, IsError: outcome.IsError})
	}
	return reply, nil
}
