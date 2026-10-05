package main

import (
	"fmt"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/internal/agent/catalog"
	"github.com/zachbornheimer/evident-output/internal/agent/rules"
)

// handleListGuides serves evident_output_list_guides.
func handleListGuides(id any, args map[string]any, cancelled *atomic.Bool) {
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
			"checksum":  catalog.Checksum()}})
}

// handleGetGuidance serves evident_output_get_guidance.
func handleGetGuidance(id any, args map[string]any, cancelled *atomic.Bool) {
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
			"truncated": truncated}})
}

// handleExplain serves evident_output_explain.
func handleExplain(id any, args map[string]any, cancelled *atomic.Bool) {
	ruleID, _ := args["rule_id"].(string)
	if r, ok := rules.Explain(ruleID); ok {
		writeRPC(id, map[string]any{
			"content": []map[string]any{{"type": "text", "text": r.Invariant}},
			"structuredContent": map[string]any{
				"schema": "evident_output.rule.v1",
				"rule":   r}})
		return
	}
	writeRPC(id, toolError("unknown rule"))
}
