package graph

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/record"
)

func TestWaitReturnsWhatTheCallbackReturned(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "fails")
	boom := errors.New("boom")
	submitWork(g, task, settlingWork(g, task, func() error { return boom }))

	if got := g.Wait(task); !errors.Is(got, boom) {
		t.Errorf("Wait = %v, want the callback's error", got)
	}
}

// settleToReturnGap is how long a callback that settled its own row takes to
// return, so a waiter woken by the settle has time to answer too early.
const settleToReturnGap = 5 * time.Millisecond

// TestWaitReturnsTheErrorACallbackReturnsAfterSettlingItsOwnRow pins that a
// waiter is answered by the callback's return value, not by the row settling
// first: Fail or Block inside the callback settles the row while the error is
// still on its way out.
func TestWaitReturnsTheErrorACallbackReturnsAfterSettlingItsOwnRow(t *testing.T) {
	for _, state := range []record.EntityState{record.Failed, record.Blocked} {
		for _, parkedFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/waiter parked first=%v", state, parkedFirst), func(t *testing.T) {
				g := New(record.NewRun())
				task := declare(g, "self-settled")
				boom := errors.New("boom")
				waiterParked := make(chan struct{})
				settled := make(chan struct{})
				submitWork(g, task, settlingWork(g, task, func() error {
					if parkedFirst {
						<-waiterParked
					}
					g.Settle(task, state)
					close(settled)
					time.Sleep(settleToReturnGap)
					return boom
				}))
				g.Kick()
				if parkedFirst {
					go func() {
						for g.Waits() == 0 {
							time.Sleep(time.Millisecond)
						}
						close(waiterParked)
					}()
				} else {
					<-settled
				}

				if got := g.Wait(task); !errors.Is(got, boom) {
					t.Errorf("Wait = %v, want the callback's error %v", got, boom)
				}
			})
		}
	}
}

func TestWaitOnASucceededTaskIsNil(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "ok")
	submitWork(g, task, settlingWork(g, task, func() error { return nil }))

	if got := g.Wait(task); got != nil {
		t.Errorf("Wait = %v, want nil", got)
	}
}

func TestWaitOnATaskNobodyDefinedIsNotStarted(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "orphan")

	got := g.Wait(task)

	if !errors.Is(got, ErrNotStarted) {
		t.Errorf("Wait = %v, want ErrNotStarted", got)
	}
}

func TestWaitNamesAFailureTheTaskStatedItself(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "self-failed")
	task.Rec.SetSummary("disk full")
	g.Settle(task, record.Failed)

	got := g.Wait(task)

	if !errors.Is(got, ErrWaitFailed) {
		t.Errorf("Wait = %v, want ErrWaitFailed", got)
	}
}

func TestWaitNamesTheReasonARowWasCancelled(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "cancelled")
	task.Rec.SetSummary("by user")
	g.Settle(task, record.Cancelled)

	got := g.Wait(task)

	if !errors.Is(got, ErrWaitCancelled) || got.Error() != ErrWaitCancelled.Error()+": by user" {
		t.Errorf("Wait = %v, want ErrWaitCancelled naming the reason", got)
	}
}

// holdingClaim is the marked function: it enters a hold first thing, as the
// code that grants a claim does.
func holdingClaim(work func()) {
	defer ProcessHolds().Enter().Leave()
	work()
}

func TestWaitIsRefusedWhileHoldingAClaim(t *testing.T) {
	sentinel := errors.New("nested")
	g := New(record.NewRun(), WithWaitUnderClaimError(sentinel))
	task := declare(g, "awaited")
	submitWork(g, task, settlingWork(g, task, func() error { return nil }))
	var got error

	holdingClaim(func() { got = g.Wait(task) })

	if !errors.Is(got, sentinel) {
		t.Errorf("Wait under a claim = %v, want the configured sentinel", got)
	}
	if task.Rec.State() == record.Done {
		t.Error("the refused Wait ran the awaited Task")
	}
}

func TestWaitContainerJoinsEveryDescendantFailureInDeclarationOrder(t *testing.T) {
	g := New(record.NewRun())
	group := g.AddContainer(nil, "group", false)
	first, second := errors.New("first"), errors.New("second")
	for _, failure := range []error{first, second} {
		member := must(g.AddTask(group, failure.Error(), record.TaskInit{State: record.Pending}))
		submitWork(g, member, settlingWork(g, member, func() error { return failure }))
	}

	got := g.WaitContainer(group)

	if !errors.Is(got, first) || !errors.Is(got, second) {
		t.Fatalf("WaitContainer = %v, want both failures", got)
	}
	if want := "first\nsecond"; got.Error() != want {
		t.Errorf("WaitContainer = %q, want %q (declaration order)", got.Error(), want)
	}
}

func TestWaitContainerOfSucceededMembersIsNil(t *testing.T) {
	g := New(record.NewRun())
	group := g.AddContainer(nil, "group", false)
	member := must(g.AddTask(group, "one", record.TaskInit{State: record.Pending}))
	submitWork(g, member, settlingWork(g, member, func() error { return nil }))

	if got := g.WaitContainer(group); got != nil {
		t.Errorf("WaitContainer = %v, want nil", got)
	}
}
