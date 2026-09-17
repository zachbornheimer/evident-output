package evo_test

import (
	"fmt"
	"io"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleSnapshot reads the immutable complete presentation state of a
// finished run.
func ExampleSnapshot() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	out.Task("apply patch").Done()
	_ = out.Finish()
	snap := out.Snapshot()
	fmt.Println(len(snap.Tasks))
	// Output:
	// 1
}

// ExampleChangesSnapshot reads an immutable changes section: the records a
// mutation verb recorded, run without DryRun.
func ExampleChangesSnapshot() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	out.Task("prune branches").Delete("stale branch", func() error { return nil }, evo.Affected(2))
	_ = out.Finish()
	changes := out.Snapshot().Changes[0]
	fmt.Println(changes.Records[0].Verb, changes.Records[0].Quantity)
	// Output:
	// deleted 2
}

// ExamplePlanSnapshot reads an immutable plan section: the records a
// mutation verb recorded under DryRun.
func ExamplePlanSnapshot() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true, DryRun: true})
	out.Task("prune branches").Delete("stale branch", func() error { return nil }, evo.Affected(2))
	_ = out.Finish()
	plan := out.Snapshot().Plans[0]
	fmt.Println(plan.Records[0].Verb, plan.Records[0].Quantity)
	// Output:
	// delete 2
}

// ExampleEffectRecord shows one semantic change or plan row.
func ExampleEffectRecord() {
	r := evo.EffectRecord{Verb: "delete", Quantity: 2, HasQty: true, Object: "stale branch"}
	fmt.Println(r.Verb, r.Quantity, r.Object)
	// Output:
	// delete 2 stale branch
}

// ExampleMessageSnapshot shows one logical user-facing message in the
// canonical model, read from a finished Snapshot.
func ExampleMessageSnapshot() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	evo.SetDefault(out)
	evo.Println("reading configuration")
	_ = out.Finish()
	msg := out.Snapshot().Messages[0]
	fmt.Println(msg.Text, msg.Visibility == evo.VisibilityNormal)
	// Output:
	// reading configuration true
}
