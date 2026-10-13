package evo

import (
	"fmt"
	"io"
)

// ExampleRedactor redacts sensitive values before journal, Capture
// retention, and human rendering.
func ExampleRedactor() {
	var r Redactor = noopRedactor_1{}
	fmt.Println(r.RedactString("token=abc123"))
	// Output:
	// token=abc123
}

// ExampleNoopRedactor leaves strings unchanged — the default Redactor.
func ExampleNoopRedactor() {
	var r noopRedactor_1
	fmt.Println(r.RedactString("token=abc123"))
	// Output:
	// token=abc123
}

// ExampleIsCharDevice reports whether w is an interactive character device
// (a real terminal) — false for an ordinary buffer or file.
func ExampleIsCharDevice() {
	fmt.Println(isCharDevice(io.Discard))
	// Output:
	// false
}

// ExamplePluralize formats a quantity with its singular noun pluralized
// when the quantity isn't exactly one.
func ExamplePluralize() {
	fmt.Println(pluralize(1, "branch"))
	fmt.Println(pluralize(3, "branch"))
	// Output:
	// branch
	// branches
}

// ExampleTruncateNames bounds a name list to visible entries, summarizing
// the rest.
func ExampleTruncateNames() {
	fmt.Println(truncateNames([]string{"main", "dev", "release", "hotfix"}, 2))
	// Output:
	// main, dev … +2 more
}

// namedTerminal is defined in example_option_test.go and reused here for
// ExampleTerminalDriver.

// fakeLiveSurface is a minimal LiveSurface for ExampleLiveSurface — real
// callers pass a concrete driver (e.g. from internal/terminal).
type fakeLiveSurface struct{}

func (fakeLiveSurface) ID() string          { return "fake" }
func (fakeLiveSurface) Columns() int        { return 80 }
func (fakeLiveSurface) Rows() int           { return 24 }
func (fakeLiveSurface) IsInteractive() bool { return false }
func (fakeLiveSurface) WriteLive(string)    {}
func (fakeLiveSurface) ClearLive()          {}
func (fakeLiveSurface) WriteDurable(string) {}
func (fakeLiveSurface) WriteFinal(string)   {}

type namedTerminal struct{ id string }

func (t namedTerminal) ID() string { return t.id }
