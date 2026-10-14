package evo

import (
	"context"
	"fmt"
	"io"
)

// ExampleSnapshot reads the immutable complete presentation state of a
// finished run.
func ExampleSnapshot() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true})
	out.Task("apply patch").Define(func(context.Context) error { return nil })
	_ = out.finish()
	snap := out.snapshot()
	fmt.Println(len(snap.Tasks))
	// Output:
	// 1
}

// ExampleChangesSnapshot reads an immutable changes section: the records a
// mutation verb recorded, run without DryRun.
func ExampleChangesSnapshot() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true})
	out.Task("prune branches").Define(effectOf(Delete, "stale branch", 2))
	_ = out.finish()
	changes := out.snapshot().Changes[0]
	fmt.Println(changes.Records[0].Verb, changes.Records[0].Quantity)
	// Output:
	// deleted 2
}

// ExamplePlanSnapshot reads an immutable plan section: the records a
// mutation verb recorded under DryRun.
func ExamplePlanSnapshot() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, DryRun: true})
	out.Task("prune branches").Define(effectOf(Delete, "stale branch", 2))
	_ = out.finish()
	plan := out.snapshot().Plans[0]
	fmt.Println(plan.Records[0].Verb, plan.Records[0].Quantity)
	// Output:
	// delete 2
}

// ExampleEffectRecord shows one semantic change or plan row.
func ExampleEffectRecord() {
	r := effectRecord{Verb: "delete", Quantity: 2, HasQty: true, Object: "stale branch"}
	fmt.Println(r.Verb, r.Quantity, r.Object)
	// Output:
	// delete 2 stale branch
}
