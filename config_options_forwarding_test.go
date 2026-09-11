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
	out := evo.Init(evo.Config{Isolated: true, DryRun: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})

	out.Task("cleanup").Delete("stale local branch", func() error { return nil }, evo.Affected(2))
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
	evo.Init(evo.Config{Isolated: true, Subject: "bpp-csharp", Stdout: &buf, Color: evo.ColorNever})

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
	out := evo.Init(evo.Config{
		Isolated: true,
		Preview:  true,
		Subject:  "repo  /tmp/flight",
		Options:  []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()},
	})

	out.Task("cleanup").Delete("stale local branch", func() error {
		called = true
		return nil
	}, evo.Affected(2))
	_ = out.Finish()

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
