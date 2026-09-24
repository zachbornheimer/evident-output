package evo_test

import (
	"fmt"
	"io"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleTaxonomyReason shows a get-or-create taxonomy Reason: the same
// string at every call site merges into one bucket.
func ExampleTaxonomyReason() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	evo.SetDefault(out)
	first := evo.Reason("dirty working tree")
	second := evo.Reason("dirty working tree")
	fmt.Println(first.Name() == second.Name())
	// Output:
	// true
}

// ExampleReason shows the get-or-create taxonomy lookup an inline
// evo.Reason("...") call always legally reuses.
func ExampleReason() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	evo.SetDefault(out)
	reason := evo.Reason("protected")
	fmt.Println(reason.Name())
	// Output:
	// protected
}

// ExampleTaxonomyRecord reads the accumulated (reason, name) disposition
// TaskHandle.Skipped records — never assembled by hand.
func ExampleTaxonomyRecord() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	evo.SetDefault(out)
	task := out.Task("branch main")
	task.Skipped(evo.Reason("protected"))
	_ = out.Finish()
	record := task.Snapshot().Skipped[0]
	fmt.Println(record.Reason, record.Name)
	// Output:
	// protected branch main
}

// ExampleForSkip shows constructing the ReasonOption that restricts a
// taxonomy Reason to TaskHandle.Skipped (recording it via Kept is misuse).
// evo.Reason itself takes no options today — this constrained-reason form is
// reachable only through the internal reasonGetOrCreate a future advanced
// entrypoint would expose; the option value is still real and constructible.
func ExampleForSkip() {
	opt := evo.ForSkip()
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleOnTask shows constructing the ReasonOption that restricts a
// taxonomy Reason to one named task (see ExampleForSkip for why it isn't
// wired through evo.Reason yet).
func ExampleOnTask() {
	opt := evo.OnTask("integration tests")
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleReasonOption shows the interface every Reason constraint
// (ForSkip, OnTask) implements.
func ExampleReasonOption() {
	opt := evo.ForSkip()
	fmt.Println(opt != nil)
	// Output:
	// true
}
