package evo

import (
	"fmt"
)

// ExampleCapture shows the retained/redacted process-output sink's zero
// value: with no owning Output attached (only reachable internally via
// Task.Writer), Write is a safe no-op rather than a panic, so an
// embedder that receives a zero Capture can always call its methods.
func exampleCapture() {
	var e capture
	n, err := e.Write([]byte("compiling module\n"))
	fmt.Println(n, err)
	fmt.Println(e.Empty())
	// Output:
	// 17 <nil>
	// true
}

// ExampleCaptureStream identifies which process stream a captured line
// came from — CaptureStreamCombined is the default (merged) stream.
func exampleCaptureStream() {
	s := captureStreamStdout
	fmt.Println(s == captureStreamStdout)
	// Output:
	// true
}

// ExampleCaptureOption shows the interface Capture construction knobs
// implement. Task.Capture accepts these options; this Example proves
// MaxCaptureBytes is a CaptureOption.
func exampleCaptureOption() {
	opt := maxCaptureBytes_1(64 << 10)
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleMaxCaptureBytes sets an approximate byte budget for Capture's
// retained lines (default 256KiB). See ExampleCaptureOption for why this
// proves construction, not effect.
func exampleMaxCaptureBytes() {
	opt := maxCaptureBytes_1(64 << 10)
	fmt.Println(opt != nil)
	// Output:
	// true
}
