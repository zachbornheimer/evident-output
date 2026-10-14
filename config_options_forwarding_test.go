package evo

import (
	"bytes"
	"context"
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

// TestInit_OptionsPath_HonorsSubject is I1: Config.Subject was silently
// dropped on the Config.Options escape hatch.
func TestInit_OptionsPath_HonorsSubject(t *testing.T) {
	var buf bytes.Buffer
	initRun(Config{Facades: Facades{Stderr: &buf}}, tuning{subtitle: "bpp-csharp", color: colorNever})

	if !strings.Contains(buf.String(), "bpp-csharp") {
		t.Fatalf("Subject not printed on the Options path, got:\n%s", buf.String())
	}
}

// TestInit_OptionsPath_HonorsPreview is the rec-dialect leak: Config.Options
// only appended preview() and never dryRun()+header, so mutations keyed off
// cfg.dryRun, callbacks ran, the ledger said [changed], and the planned
// header never printed. Ordinary Config{Preview: true} was already honest.
func TestInit_OptionsPath_HonorsPreview(t *testing.T) {
	called := false
	var buf bytes.Buffer
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{preview: true, subtitle: "repo  /tmp/flight", color: colorNever})

	out.Task("cleanup").Define(func(ctx context.Context) error {
		return Effect(ctx, EffectSpec{Verb: Delete, Object: "stale local branch", Quantity: 2}, func(context.Context) error {
			called = true
			return nil
		})
	})
	_ = out.finish()

	got := buf.String()
	if called {
		t.Fatalf("Preview on the Options path must not run callbacks:\n%s", got)
	}
	if strings.Contains(got, "[changed]") {
		t.Fatalf("Preview on the Options path must not use committed tense:\n%s", got)
	}
	if strings.Contains(got, "[dry-run]") {
		t.Fatalf("Preview is not a dry run:\n%s", got)
	}
	if !strings.Contains(got, "repo  /tmp/flight") {
		t.Fatalf("Preview on the Options path must render the planned header:\n%s", got)
	}
	if !strings.Contains(got, "planned") {
		t.Fatalf("Preview on the Options path must plan, got:\n%s", got)
	}
}
