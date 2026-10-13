package main

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/record"
	"github.com/zachbornheimer/evident-output/mcp/internal/agent/preview"
)

// handlePreview serves evident_output_preview.
func handlePreview(id any, args map[string]any, cancelled *atomic.Bool) {
	subject, _ := args["subject"].(string)
	item, _ := args["item"].(string)
	state, _ := args["state"].(string)
	dbg, _ := args["debug"].(string)
	if subject == "" {
		subject = "demo"
	}
	if item == "" {
		item = "status"
	}
	var buf bytes.Buffer
	_ = evo.Execute(context.Background(), evo.Config{
		Name:    subject,
		Output:  evo.Plain,
		Facades: evo.Facades{Stdout: &buf},
	}, func(ctx context.Context, r *evo.Run) error {
		it := r.Task(item)
		switch state {
		case "blocked":
			it.Define(func(context.Context) error { return evo.Refuse("blocked for demo") })
		case "failed":
			it.Define(func(context.Context) error { return evo.Fail("failed for demo") })
		default:
			it.Define(func(context.Context) error { return nil })
		}
		return nil
	})
	_ = dbg
	profiles := preview.DefaultProfiles(record.Snapshot{})
	if cancelled.Load() {
		writeRPC(id, toolError("deadline exceeded"))
		return
	}
	writeRPC(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("%d profiles", len(profiles))}},
		"structuredContent": map[string]any{
			"schema":   "evident_output_preview.v1",
			"profiles": profiles,
			"plain":    buf.String(),
		},
	})
}
