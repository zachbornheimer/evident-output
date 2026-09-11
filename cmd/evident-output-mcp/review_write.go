package main

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

const (
	nextActionClean             = "clean: 0 findings, no recheck needed"
	nextActionReviewLoop        = "re-run evident_output_review after applying the suggested fixes, until findings=0 and recheck_required=false"
	nextActionUpdateThenRestart = "call evident_output_update then restart the MCP host before treating this review as done"
)

func writeReviewResult(id any, res review.Result) {
	needed := updateNeeded(reviewIdentity(), pinForReview(reviewPin{
		DesiredVersion: res.DesiredVersion,
		ModuleVersion:  res.ModuleVersion,
		ReplacePath:    res.ReplacePath,
	}))
	nextAction := reviewNextAction(res, needed)
	text := fmt.Sprintf("findings=%d recheck=%v partial=%v — %s", len(res.Findings), res.RecheckRequired, res.Partial, nextAction)
	writeRPC(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"structuredContent": map[string]any{
			"schema":           "evident_output_review.v1",
			"recheck_required": res.RecheckRequired,
			"partial":          res.Partial,
			"findings":         res.Findings,
			"next_action":      nextAction,
			"mcp_version":      reviewIdentity().Version,
			"desired_version":  res.DesiredVersion,
			"module_version":   res.ModuleVersion,
			"replace_path":     res.ReplacePath,
			"update_needed":    needed,
		}})
}

func reviewNextAction(res review.Result, updateNeeded bool) string {
	loop := nextActionReviewLoop
	if len(res.Findings) == 0 && !res.RecheckRequired {
		loop = nextActionClean
	}
	if updateNeeded {
		if loop == nextActionClean {
			return nextActionUpdateThenRestart
		}
		return nextActionUpdateThenRestart + "; " + loop
	}
	return loop
}
