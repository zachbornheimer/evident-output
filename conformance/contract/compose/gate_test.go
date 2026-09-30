package compose_test

import (
	"context"
	"sync/atomic"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// The Blocked-gate idiom (ZYS-1345): a plain Task Blocks and returns nil.
func TestC31_007_BlockedGateLeavesLaterStepsNotStartedAndSiblingCompletes(t *testing.T) {
	out := newQuietOutput(t, false)
	var laterRan, siblingRan atomic.Bool
	steps := out.Sequence("deploy")
	gate := steps.Task("check deploy policy")
	gate.Define(func(context.Context) error {
		gate.Block("deploys are frozen")
		return nil
	})
	later := steps.Task("ship release")
	later.Define(func(context.Context) error { laterRan.Store(true); return nil })

	sibling := out.Group("housekeeping").Task("prune stale branches")
	sibling.Define(func(context.Context) error { siblingRan.Store(true); return nil })

	_ = out.Finish()
	if laterRan.Load() || later.Snapshot().State != evo.NotStarted {
		t.Fatalf("later step ran=%v state=%v, want never started", laterRan.Load(), later.Snapshot().State)
	}
	if !siblingRan.Load() || sibling.Snapshot().State != evo.Done {
		t.Fatal("an independent Group sibling must still complete")
	}
}
