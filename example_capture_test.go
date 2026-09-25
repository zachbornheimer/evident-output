package evo_test

import (
	"fmt"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleCapture shows the retained/redacted process-output sink's zero
// value: with no owning Output attached (only reachable internally via
// TaskHandle.Writer), Write is a safe no-op rather than a panic, so an
// embedder that receives a zero Capture can always call its methods.
func ExampleCapture() {
	var e evo.Capture
	n, err := e.Write([]byte("compiling module\n"))
	fmt.Println(n, err)
	fmt.Println(e.Empty())
	// Output:
	// 17 <nil>
	// true
}

// ExampleCaptureStream identifies which process stream a captured line
// came from — CaptureStreamCombined is the default (merged) stream.
func ExampleCaptureStream() {
	s := evo.CaptureStreamStdout
	fmt.Println(s == evo.CaptureStreamStdout)
	// Output:
	// true
}

// ExampleCaptureOption shows the interface every Capture construction
// knob (KeepLastLines, MaxCaptureBytes, MirrorToDebug,
// MirrorToDiagnostics) implements. Capture itself has no exported
// constructor accepting these today — the option value is real and
// constructible, but its effect isn't reachable from the public surface
// yet, so this Example proves construction rather than behavior.
func ExampleCaptureOption() {
	opt := evo.KeepLastLines(50)
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleKeepLastLines sets how many trailing lines Capture retains
// (default 200). See ExampleCaptureOption for why this proves
// construction, not effect.
func ExampleKeepLastLines() {
	opt := evo.KeepLastLines(50)
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleMaxCaptureBytes sets an approximate byte budget for Capture's
// retained lines (default 256KiB). See ExampleCaptureOption for why this
// proves construction, not effect.
func ExampleMaxCaptureBytes() {
	opt := evo.MaxCaptureBytes(64 << 10)
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleMirrorToDiagnostics copies each completed Capture line to the
// Diagnostics writer. See ExampleCaptureOption for why this proves
// construction, not effect.
func ExampleMirrorToDiagnostics() {
	opt := evo.MirrorToDiagnostics()
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleMirrorToDebug journals each completed Capture line via Debug when
// DebugLevel allows. See ExampleCaptureOption for why this proves
// construction, not effect.
func ExampleMirrorToDebug() {
	opt := evo.MirrorToDebug()
	fmt.Println(opt != nil)
	// Output:
	// true
}
