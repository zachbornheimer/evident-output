package evo_test

import (
	"fmt"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleDebugConfig configures the debug journal presentation: the minimum
// level and whether it renders as durable history or a bounded pane.
func ExampleDebugConfig() {
	cfg := evo.DebugConfig{Level: evo.LevelDebug, View: evo.DebugPresentationHistory}
	fmt.Println(cfg.Level == evo.LevelDebug, cfg.View == evo.DebugPresentationHistory)
	// Output:
	// true true
}

// ExampleDebugPresentation selects history vs pane presentation for the
// debug journal (default History). Pane presentation is configured with
// Config.Debug's View, PaneHeight, NewestFirst, and PreserveAlways fields —
// see ExampleConfig_debugPane.
func ExampleDebugPresentation() {
	view := evo.DebugPresentationPane
	fmt.Println(view == evo.DebugPresentationPane)
	// Output:
	// true
}

// ExampleDebugPaneOption shows the interface NewestFirst, OldestFirst,
// PaneHeight, and PreserveDebugTail each implement — retained as build-once
// values for embedders composing their own presentation layer; ordinary
// callers set Config.Debug's View/PaneHeight/NewestFirst/PreserveAlways
// fields directly (see ExampleConfig_debugPane).
func ExampleDebugPaneOption() {
	opt := evo.PaneHeight(5)
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleNewestFirst builds the newest-first pane ordering value.
func ExampleNewestFirst() {
	opt := evo.NewestFirst()
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleOldestFirst builds the oldest-first pane ordering value.
func ExampleOldestFirst() {
	opt := evo.OldestFirst()
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExamplePaneHeight builds a bounded pane-height value (default 5).
func ExamplePaneHeight() {
	opt := evo.PaneHeight(3)
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExamplePreserveDebugTail builds the always-preserve-tail value.
func ExamplePreserveDebugTail() {
	opt := evo.PreserveDebugTail()
	fmt.Println(opt != nil)
	// Output:
	// true
}
