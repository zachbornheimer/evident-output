package evo

import (
	"bytes"
	"strings"
	"testing"
)

// TestInit_OptionsPath_HonorsDryRun is I1: Config.DryRun was silently
// dropped on the Config.Options escape hatch — a caller combining DryRun
// with explicit Options got [changed] rows instead of [planned].
func TestInit_OptionsPath_HonorsDryRun(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{DryRun: true, Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})

	out.Task("cleanup").Define(effectOf(Delete, "stale local branch", 2))
	_ = out.finish()

	rendered := buf.String()
	if !strings.Contains(rendered, "planned") {
		t.Fatalf("DryRun not honored on the Options path, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "changed") {
		t.Fatalf("expected no [changed] row under DryRun, got:\n%s", rendered)
	}
}
