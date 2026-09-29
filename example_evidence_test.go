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

// ExampleCaptureOption shows the interface Capture construction knobs
// implement. TaskHandle.Capture accepts these options; this Example proves
// MaxCaptureBytes is a CaptureOption.
func ExampleCaptureOption() {
	opt := evo.MaxCaptureBytes(64 << 10)
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
