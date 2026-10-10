package graph

import (
	"errors"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// blockedWork is submitted work that stays unstarted: nothing kicks it.
func blockedWork() Work { return Work{Run: func() error { return nil }} }

func TestAdmitRefusesATaskThatAlreadySettled(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "settled")
	g.Settle(task, record.Failed)

	got := g.Admit(task, Resolution{State: record.Done, By: ByCaller})

	if got.Verdict != AlreadyResolved {
		t.Errorf("Verdict = %v, want AlreadyResolved", got.Verdict)
	}
}

func TestObserveRatifiesTheProposalAReturnedNilBacks(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "proposing")
	submitWork(g, task, blockedWork())
	g.Admit(task, Resolution{State: record.Done, Summary: "8 deleted", By: ByCaller})

	got := g.Observe(task, nil)

	if got.Kind != ObservedProposal || got.Proposal == nil || got.Proposal.Summary != "8 deleted" {
		t.Errorf("Observe = %+v, want the proposal ratified", got)
	}
}

func TestObserveDiscardsTheProposalAReturnedErrorContradicts(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "proposing")
	submitWork(g, task, blockedWork())
	g.Admit(task, Resolution{State: record.Done, By: ByCaller})

	got := g.Observe(task, errors.New("boom"))

	if got.Kind != ObservedFailure || got.Failure != "boom" {
		t.Errorf("Observe = %+v, want a failure carrying boom", got)
	}
	if task.HasProposal() {
		t.Error("the contradicted proposal was kept")
	}
}

func TestObserveOfANilReturnWithNoProposalIsSuccess(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "plain")

	if got := g.Observe(task, nil); got.Kind != ObservedSuccess {
		t.Errorf("Observe = %+v, want success", got)
	}
}

func TestConcludeLeavesATaskAnInterruptSettledBetweenAdmitAndConclude(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "submitted")
	submitWork(g, task, blockedWork())
	admitted := g.Admit(task, Resolution{State: record.Failed, By: ByCaller})

	g.Settle(task, record.Cancelled)
	g.Conclude(task, admitted)

	if got := task.Rec.State(); got != record.Cancelled {
		t.Errorf("state = %s, want cancelled: Conclude settled the Task a second time", got)
	}
}

func TestAdmitHoldsACallersSuccessOnSubmittedWorkAsAProposal(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "submitted")
	submitWork(g, task, blockedWork())

	got := g.Admit(task, Resolution{State: record.Done, Summary: "all good", By: ByCaller})

	if got.Verdict != Proposed {
		t.Fatalf("Verdict = %v, want Proposed", got.Verdict)
	}
	if record.IsTerminalTask(task.Rec.State()) {
		t.Error("a proposal settled the Task")
	}
	proposal := task.TakeProposal()
	if proposal == nil || proposal.State != record.Done || proposal.Summary != "all good" {
		t.Errorf("held proposal = %+v, want Done \"all good\"", proposal)
	}
}

func TestAdmitSettlesACallersFailureOnSubmittedWorkAtOnce(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "submitted")
	submitWork(g, task, blockedWork())

	got := g.Admit(task, Resolution{State: record.Failed, By: ByCaller})

	if got.Verdict != Settling || got.State != record.Failed {
		t.Errorf("Admission = %+v, want Settling as Failed", got)
	}
}

func TestAdmitLetsTheSchedulerDeclareSubmittedWorkDone(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "submitted")
	submitWork(g, task, blockedWork())

	got := g.Admit(task, Resolution{State: record.Done, By: ByScheduler})

	if got.Verdict != Settling || got.State != record.Done {
		t.Errorf("Admission = %+v, want Settling as Done", got)
	}
}

func TestConcludeSettlesTheTaskAsAdmitted(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "plain")

	g.Conclude(task, g.Admit(task, Resolution{State: record.Skipped, By: ByCaller}))

	if got := task.Rec.State(); got != record.Skipped {
		t.Errorf("state = %s, want skipped", got)
	}
}

func TestConcludeIgnoresAnAdmissionThatDidNotSettle(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "submitted")
	submitWork(g, task, blockedWork())

	g.Conclude(task, g.Admit(task, Resolution{State: record.Done, By: ByCaller}))

	if record.IsTerminalTask(task.Rec.State()) {
		t.Error("Conclude settled a Task whose success was only proposed")
	}
}

func TestConcludeStopsTheStepsAfterAFailedSubmittedStep(t *testing.T) {
	g := New(record.NewRun())
	steps := g.AddContainer(nil, "steps", true)
	first := must(g.AddTask(steps, "first", record.TaskInit{State: record.Pending}))
	second := must(g.AddTask(steps, "second", record.TaskInit{State: record.Pending}))
	submitWork(g, first, blockedWork())
	submitWork(g, second, blockedWork())

	g.Conclude(first, g.Admit(first, Resolution{State: record.Failed, By: ByCaller}))

	if got := second.Rec.State(); got != record.NotStarted {
		t.Errorf("second = %s, want not started once the step before it failed", got)
	}
}
