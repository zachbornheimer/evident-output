package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestInit_OptionsPath_HonorsDryRun is I1: Config.DryRun was silently
// dropped on the Config.Options escape hatch — a caller combining DryRun
// with explicit Options got [changed] rows instead of [planned].
func TestInit_OptionsPath_HonorsDryRun(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		DryRun:  true,
		Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()},
	})

	out.Task("cleanup").Delete(2, "stale local branches")
	_ = out.Finish()

	rendered := buf.String()
	if !strings.Contains(rendered, "planned") {
		t.Fatalf("DryRun not honored on the Options path, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "changed") {
		t.Fatalf("expected no [changed] row under DryRun, got:\n%s", rendered)
	}
}

// TestInit_OptionsPath_HonorsSubject is I1: Config.Subject was silently
// dropped on the Config.Options escape hatch.
func TestInit_OptionsPath_HonorsSubject(t *testing.T) {
	var buf bytes.Buffer
	evo.Init(evo.Config{
		Subject: "bpp-csharp",
		Options: []evo.Option{evo.To(&buf), evo.NoColor()},
	})

	if !strings.Contains(buf.String(), "bpp-csharp") {
		t.Fatalf("Subject not printed on the Options path, got:\n%s", buf.String())
	}
}
