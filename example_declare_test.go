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

// ExampleMutationOption shows the interface Affected implements, configuring
// how many objects one atomic mutation touches.
func ExampleMutationOption() {
	opt := evo.Affected(3)
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleAffected sets how many objects one atomic mutation touches — omit
// it for a Task that affects a single item.
func ExampleAffected() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	task := out.Task("prune branches")
	task.Delete("stale branches", func() error { return nil }, evo.Affected(5))
	_ = task.Wait()
	_ = out.Finish()
	fmt.Println(task.Snapshot().State)
	// Output:
	// done
}

// ExampleEntityOption shows the interface ID and StartPhase implement — an
// advanced, platform-scale Task-declaration configuration surface.
func ExampleEntityOption() {
	opt := evo.ID("download-base-image")
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleID sets a stable machine key independent of a Task's human label
// (superseded today: Task is name-only, kept for advanced/platform callers
// building their own declaration layer over EntityOption).
func ExampleID() {
	opt := evo.ID("download-base-image")
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleStartPhase sets a task's first doing-text at declare time
// (superseded today: call TaskHandle.Doing after declaring instead).
func ExampleStartPhase() {
	opt := evo.StartPhase("resolving dependencies")
	fmt.Println(opt != nil)
	// Output:
	// true
}
