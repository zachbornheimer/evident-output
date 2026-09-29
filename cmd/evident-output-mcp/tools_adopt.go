package main

import (
	"fmt"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/internal/agent/adopt"
)

// handleAdoptPlan serves evident_output_adopt_plan.
func handleAdoptPlan(id any, args map[string]any, cancelled *atomic.Bool) {
	directory, _ := args["directory"].(string)
	if directory == "" {
		writeRPC(id, toolError("directory is required"))
		return
	}
	if isRemotePath(directory) {
		writeRPC(id, toolError("remote path unsupported; pass a local directory (MCP-036)"))
		return
	}
	cursor, _ := args["cursor"].(string)
	page, err := adopt.InventoryPage(directory, adopt.InventoryOptions{
		Cursor: cursor,
		Limit:  intFromArgs(args, "limit")})
	if err != nil {
		writeRPC(id, toolError("adopt_plan: "+err.Error()))
		return
	}
	if cancelled.Load() {
		writeRPC(id, toolError("deadline exceeded"))
		return
	}
	writeRPC(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("%d findings, remaining=%d — %s", len(page.Findings), page.Remaining, page.NextAction)}},
		"structuredContent": map[string]any{
			"schema":      "evident_output_adopt_plan.v1",
			"directory":   page.Directory,
			"findings":    page.Findings,
			"rung":        page.Rung,
			"remaining":   page.Remaining,
			"next_cursor": page.NextCursor,
			"next_action": page.NextAction,
			"facades":     page.Facades,
			"caveat":      page.Caveat}})
}
