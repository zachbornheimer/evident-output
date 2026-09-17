package evo_test

import (
	"bytes"
	"fmt"
	"io"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleProblem shows the structured evidence shape explaining a negative
// task outcome — the payload Fail/Block/Warn build from ProblemOptions.
func ExampleProblem() {
	p := evo.Problem{Summary: "schema mismatch", Code: "E_SCHEMA"}
	fmt.Println(p.Summary, p.Code)
	// Output:
	// schema mismatch E_SCHEMA
}

// ExampleSourceLocation shows a path-based source position attached to a
// Problem via Location.
func ExampleSourceLocation() {
	loc := evo.SourceLocation{Path: "config.yaml", Line: 12, Column: 3}
	fmt.Println(loc.Path, loc.Line, loc.Column)
	// Output:
	// config.yaml 12 3
}

// ExampleAttachment shows an additional label/value fact attached to a
// Problem — a different concept from the Evidence retention sink.
func ExampleAttachment() {
	a := evo.Attachment{Label: "stderr", Value: "permission denied"}
	fmt.Println(a.Label, a.Value)
	// Output:
	// stderr permission denied
}

// ExampleField shows a structured diagnostic or log field.
func ExampleField() {
	f := evo.Field{Key: "retries", Value: 3}
	fmt.Println(f.Key, f.Value)
	// Output:
	// retries 3
}

// ExampleProblemOption shows the interface every Problem-configuring
// constructor (Detail, Code, On, Count, Location, Next, NextCommand)
// implements.
func ExampleProblemOption() {
	opt := evo.Detail("the file was not found")
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleDetail sets user-visible detail text on a Problem raised via
// TaskHandle.Fail.
func ExampleDetail() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: &buf, Stderr: io.Discard})
	out.Task("read config").Fail("parse error", evo.Detail("unexpected token at line 4"))
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✗ read config  parse error
	//    └─ unexpected token at line 4
}

// ExampleCode sets a stable, machine-readable Problem code a consumer can
// match on instead of parsing Summary text.
func ExampleCode() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: &buf, Stderr: io.Discard})
	out.Task("apply migration").Fail("schema mismatch", evo.Code("E_SCHEMA"))
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✗ apply migration  schema mismatch
}

// ExampleOn sets the problem subject.
func ExampleOn() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: &buf, Stderr: io.Discard})
	out.Task("clean workspace").Fail("permission denied", evo.On("/var/cache"))
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✗ clean workspace  permission denied
	//    ├─ /var/cache  permission denied
}

// ExampleCount sets a quantity and optional unit on a Problem.
func ExampleCount() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: &buf, Stderr: io.Discard})
	out.Task("upload artifacts").Fail("upload failed", evo.Count(3, "files"))
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✗ upload artifacts  upload failed
}

// ExampleLocation sets a source location on a Problem.
func ExampleLocation() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: &buf, Stderr: io.Discard})
	out.Task("validate manifest").Fail("unknown field", evo.Location("manifest.yaml", 4, 1))
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✗ validate manifest  unknown field
}

// ExampleNext attaches a recommended next step to a Problem.
func ExampleNext() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: &buf, Stderr: io.Discard})
	out.Task("push branch").Fail("rejected: non-fast-forward", evo.Next(evo.Label("pull --rebase first")))
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✗ push branch  rejected: non-fast-forward
}

// ExampleNextCommand attaches a recommended command action to a Problem.
func ExampleNextCommand() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: &buf, Stderr: io.Discard})
	out.Task("push branch").Fail("rejected: non-fast-forward", evo.NextCommand("git", "pull", "--rebase"))
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✗ push branch  rejected: non-fast-forward
}

// ExampleFailure shows the value TaskHandle.Failf/Blockf return: one
// recorded error built and returned in a single line, further chainable
// with Next/NextCommand.
func ExampleFailure() {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Stderr: io.Discard, Plain: true})
	task := out.Task("clone repository")
	err := task.Failf("clone failed: %w", fmt.Errorf("connection refused"))
	_ = out.Finish()
	fmt.Println(err.Error())
	// Output:
	// clone failed: connection refused
}
