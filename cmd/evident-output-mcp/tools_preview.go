package main

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/agent/preview"
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
	out := evo.Init(evo.Config{Title: subject, Stdout: &buf, Plain: true, Color: evo.ColorNever, Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	it := out.Task(item)
	switch state {
	case "blocked":
		it.Block("blocked for demo")
	case "failed":
		it.Fail("failed for demo")
	default:
		it.Define(func(context.Context) error { return nil })
	}
	_ = dbg
	_ = out.Finish()
	snap := out.Snapshot()
	profiles := preview.DefaultProfiles(snap)
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
