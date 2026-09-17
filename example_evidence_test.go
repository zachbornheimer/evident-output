package evo_test

import (
	"fmt"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleEvidence shows the retained/redacted process-output sink's zero
// value: with no owning Output attached (only reachable internally via
// TaskHandle.Writer), Write is a safe no-op rather than a panic, so an
// embedder that receives a zero Evidence can always call its methods.
func ExampleEvidence() {
	var e evo.Evidence
	n, err := e.Write([]byte("compiling module\n"))
	fmt.Println(n, err)
	fmt.Println(e.Empty())
	// Output:
	// 17 <nil>
	// true
}

// ExampleEvidenceStream identifies which process stream a captured line
// came from — EvidenceStreamCombined is the default (merged) stream.
func ExampleEvidenceStream() {
	s := evo.EvidenceStreamStdout
	fmt.Println(s == evo.EvidenceStreamStdout)
	// Output:
	// true
}

// ExampleEvidenceOption shows the interface every Evidence construction
// knob (KeepLastLines, MaxEvidenceBytes, MirrorToDebug,
// MirrorToDiagnostics) implements. Evidence itself has no exported
// constructor accepting these today — the option value is real and
// constructible, but its effect isn't reachable from the public surface
// yet, so this Example proves construction rather than behavior.
func ExampleEvidenceOption() {
	opt := evo.KeepLastLines(50)
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleKeepLastLines sets how many trailing lines Evidence retains
// (default 200). See ExampleEvidenceOption for why this proves
// construction, not effect.
func ExampleKeepLastLines() {
	opt := evo.KeepLastLines(50)
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleMaxEvidenceBytes sets an approximate byte budget for Evidence's
// retained lines (default 256KiB). See ExampleEvidenceOption for why this
// proves construction, not effect.
func ExampleMaxEvidenceBytes() {
	opt := evo.MaxEvidenceBytes(64 << 10)
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleMirrorToDiagnostics copies each completed Evidence line to the
// Diagnostics writer. See ExampleEvidenceOption for why this proves
// construction, not effect.
func ExampleMirrorToDiagnostics() {
	opt := evo.MirrorToDiagnostics()
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleMirrorToDebug journals each completed Evidence line via Debug when
// DebugLevel allows. See ExampleEvidenceOption for why this proves
// construction, not effect.
func ExampleMirrorToDebug() {
	opt := evo.MirrorToDebug()
	fmt.Println(opt != nil)
	// Output:
	// true
}
