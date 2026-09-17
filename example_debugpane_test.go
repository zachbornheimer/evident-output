package evo_test

import (
	"bytes"
	"fmt"
	"io"

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
// debug journal (default History).
func ExampleDebugPresentation() {
	view := evo.DebugPresentationPane
	fmt.Println(view == evo.DebugPresentationPane)
	// Output:
	// true
}

// ExampleDebugPaneOption shows the interface every debug-pane knob
// (NewestFirst, OldestFirst, PaneHeight, PreserveDebugTail) implements.
func ExampleDebugPaneOption() {
	opt := evo.PaneHeight(5)
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleNewestFirst orders a debug pane newest-entry-first.
func ExampleNewestFirst() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Stdout: &buf, Stderr: io.Discard,
		Debug:   evo.DebugConfig{Level: evo.LevelDebug, View: evo.DebugPresentationPane},
		Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DebugPane(evo.NewestFirst())},
	})
	out.Task("demo").Done()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ demo
}

// ExampleOldestFirst orders a debug pane oldest-entry-first.
func ExampleOldestFirst() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Stdout: io.Discard, Stderr: io.Discard,
		Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DebugPane(evo.OldestFirst())},
	})
	out.Task("demo").Done()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ demo
}

// ExamplePaneHeight bounds a debug pane's visible line count (default 5).
func ExamplePaneHeight() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Stdout: io.Discard, Stderr: io.Discard,
		Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DebugPane(evo.PaneHeight(3))},
	})
	out.Task("demo").Done()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ demo
}

// ExamplePreserveDebugTail forces a diagnostic tail on every Finish in pane
// mode.
func ExamplePreserveDebugTail() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Stdout: io.Discard, Stderr: io.Discard,
		Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DebugPane(evo.PreserveDebugTail())},
	})
	out.Task("demo").Done()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ demo
}
