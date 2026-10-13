package evo

import (
	"bytes"
	"context"
	"fmt"
	"io"
)

// ExampleDebugConfig configures the debug journal presentation: the minimum
// level and whether it renders as durable history or a bounded pane.
func exampleDebugConfig() {
	cfg := debugConfig{level: levelDebug, view: debugPresentationHistory}
	fmt.Println(cfg.level == levelDebug, cfg.view == debugPresentationHistory)
	// Output:
	// true true
}

// ExampleDebugPresentation selects history vs pane presentation for the
// debug journal (default History).
func exampleDebugPresentation() {
	view := debugPresentationPane
	fmt.Println(view == debugPresentationPane)
	// Output:
	// true
}

// ExampleDebugPaneOption shows the interface every debug-pane knob
// (NewestFirst, OldestFirst, PaneHeight, PreserveDebugTail) implements.
func exampleDebugPaneOption() {
	opt := paneHeight_1(5)
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleNewestFirst orders a debug pane newest-entry-first.
func exampleNewestFirst() {
	var buf bytes.Buffer
	out := initRun(Config{
		isolated: true, plain: true, stdout: &buf, stderr: io.Discard, color: colorNever,
		Debug: debugConfig{level: levelDebug, view: debugPresentationPane, newestFirst: boolPtr(true)},
	})
	out.Task("demo").Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ demo
}

// ExampleOldestFirst orders a debug pane oldest-entry-first.
func exampleOldestFirst() {
	var buf bytes.Buffer
	out := initRun(Config{
		isolated: true, stdout: &buf, stderr: io.Discard, plain: true, color: colorNever,
		Debug: debugConfig{view: debugPresentationPane, newestFirst: boolPtr(false)},
	})
	out.Task("demo").Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ demo
}

// ExamplePaneHeight bounds a debug pane's visible line count (default 5).
func examplePaneHeight() {
	var buf bytes.Buffer
	out := initRun(Config{
		isolated: true, stdout: &buf, stderr: io.Discard, plain: true, color: colorNever,
		Debug: debugConfig{view: debugPresentationPane, paneHeight: 3},
	})
	out.Task("demo").Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ demo
}

// ExamplePreserveDebugTail forces a diagnostic tail on every Finish in pane
// mode.
func examplePreserveDebugTail() {
	var buf bytes.Buffer
	out := initRun(Config{
		isolated: true, stdout: &buf, stderr: io.Discard, plain: true, color: colorNever,
		Debug: debugConfig{view: debugPresentationPane, preserveAlways: true},
	})
	out.Task("demo").Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ demo
}

func boolPtr(v bool) *bool { return &v }
