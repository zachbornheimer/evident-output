package main

import (
	"fmt"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/internal/agent/sections"
)

// handleListSections serves evident_output_list_sections.
func handleListSections(id any, args map[string]any, cancelled *atomic.Bool) {
	query, _ := args["query"].(string)
	list := sections.Filter(query)
	if cancelled.Load() {
		writeRPC(id, toolError("deadline exceeded"))
		return
	}
	writeRPC(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("%d sections", len(list))}},
		"structuredContent": map[string]any{
			"schema":   "evident_output.sections.v1",
			"sections": summarizeSections(list)}})
}

// handleGetDocumentation serves evident_output_get_documentation.
func handleGetDocumentation(id any, args map[string]any, cancelled *atomic.Bool) {
	var ids []string
	if raw, ok := args["ids"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				ids = append(ids, s)
			}
		}
	}
	var found []sections.Section
	var missing []string
	for _, sid := range ids {
		if s, ok := sections.Get(sid); ok {
			found = append(found, s)
		} else {
			missing = append(missing, sid)
		}
	}
	if cancelled.Load() {
		writeRPC(id, toolError("deadline exceeded"))
		return
	}
	writeRPC(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("found=%d missing=%d", len(found), len(missing))}},
		"structuredContent": map[string]any{
			"schema":   "evident_output.documentation.v1",
			"sections": found,
			"missing":  missing}})
}

// summarizeSections strips body text for the list view — evident_output_list_sections
// is a table of contents; evident_output_get_documentation returns the body.
func summarizeSections(list []sections.Section) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, s := range list {
		out = append(out, map[string]any{
			"id":       s.ID,
			"title":    s.Title,
			"source":   s.Source,
			"concepts": s.Concepts})
	}
	return out
}
