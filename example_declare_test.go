package evo

import (
	"fmt"
	"io"
)

// ExampleTaxonomyReason shows a get-or-create taxonomy Reason: the same
// string at every call site merges into one bucket.
func ExampleTaxonomyReason() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	setDefault(out)
	first := reason("dirty working tree")
	second := reason("dirty working tree")
	fmt.Println(first.Name() == second.Name())
	// Output:
	// true
}

// ExampleReason shows the get-or-create taxonomy lookup an inline
// Reason("...") call always legally reuses.
func exampleReason() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	setDefault(out)
	reason := reason("protected")
	fmt.Println(reason.Name())
	// Output:
	// protected
}

// ExampleTaxonomyRecord reads the accumulated (reason, name) disposition
// Task.Skipped records — never assembled by hand.
func ExampleTaxonomyRecord() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	setDefault(out)
	task := out.Task("branch main")
	task.skipped(reason("protected"))
	_ = out.finish()
	record := task.snapshot().Skipped[0]
	fmt.Println(record.Reason, record.Name)
	// Output:
	// protected branch main
}
