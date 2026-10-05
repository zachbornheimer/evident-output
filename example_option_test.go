package evo_test

import (
	"bytes"
	"context"
	"fmt"
	"io"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleDryRun declares the run a dry run: mutation verbs render as
// [planned] rows with the imperative verb instead of [changed] rows.
func ExampleConfig_dryRun() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: &buf, Stderr: io.Discard, DryRun: true})
	out.Task("prune branches").Define(effectOf(evo.EffectDelete, "stale branch", 3))
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// [dry-run] no changes will be made
	//
	// ✓ prune branches
	//
	// [planned] prune branches  delete 3 stale branches
	//
	// [planned]
}

// ExampleTitle sets the subject shown in the conclusion band.
func ExampleTitle() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: &buf, Stderr: io.Discard, Title: "repo-retire"})
	out.Task("scan").Define(func(context.Context) error { return nil })
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ scan
	//
	// [ready]  repo-retire
}

// ExampleGlyphProfile shows the vocabulary selector Config.Glyphs takes: the
// zero value, GlyphsAuto, detects Unicode-vs-ASCII from locale and
// interactivity.
func ExampleGlyphProfile() {
	profile := evo.GlyphsUnicode
	fmt.Println(profile == evo.GlyphsUnicode)
	// Output:
	// true
}
