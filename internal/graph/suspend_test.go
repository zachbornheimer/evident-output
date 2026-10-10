package graph

import (
	"errors"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// suspendQuiet is how long a test watches for something that must not happen.
const suspendQuiet = 50 * time.Millisecond

func TestSuspendStartsNothingNewUntilTheWindowEnds(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "queued")
	submitWork(g, task, settlingWork(g, task, func() error { return nil }))

	_ = g.Suspend(func() error {
		g.Kick()
		time.Sleep(suspendQuiet)
		if g.Executing() != 0 || task.Rec.State() != record.Pending {
			t.Errorf("a Task started inside the window: executing %d, state %s", g.Executing(), task.Rec.State())
		}
		return nil
	})

	g.Drain()
	if got := task.Rec.State(); got != record.Done {
		t.Errorf("state after the window = %s, want done", got)
	}
}

func TestSuspendReturnsWhatItsFuncReturned(t *testing.T) {
	g := New(record.NewRun())
	boom := errors.New("boom")

	if got := g.Suspend(func() error { return boom }); !errors.Is(got, boom) {
		t.Errorf("Suspend = %v, want the func's error", got)
	}
	if g.Suspended() {
		t.Error("the window stayed open after fn returned")
	}
}

func TestSuspendWindowsNestAndResumeWhenTheLastEnds(t *testing.T) {
	g := New(record.NewRun())

	_ = g.Suspend(func() error {
		_ = g.Suspend(func() error { return nil })
		if !g.Suspended() {
			t.Error("the inner window ending resumed the outer one")
		}
		return nil
	})

	if g.Suspended() {
		t.Error("the run stayed suspended after the last window ended")
	}
}

func TestSuspendDoesNotReleaseAParkedWaitAsDeadlocked(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "awaited")
	submitWork(g, task, settlingWork(g, task, func() error { return nil }))
	waited := make(chan error, 1)

	_ = g.Suspend(func() error {
		go func() { waited <- g.Wait(task) }()
		for g.Waits() == 0 {
			time.Sleep(time.Millisecond)
		}
		g.Kick()
		select {
		case got := <-waited:
			t.Errorf("Wait returned %v inside the window, want it parked", got)
		case <-time.After(suspendQuiet):
		}
		return nil
	})

	select {
	case got := <-waited:
		if got != nil {
			t.Errorf("Wait = %v after the window, want nil", got)
		}
	case <-time.After(executorTimeout):
		t.Fatal("Wait never returned after the window ended")
	}
}

func TestAnswerOutcomeDecidesTheGateState(t *testing.T) {
	cases := map[Answer]record.EntityState{
		AnswerYes:         record.Done,
		AnswerAssumedYes:  record.Done,
		AnswerNo:          record.Blocked,
		AnswerPolicy:      record.Blocked,
		AnswerClosed:      record.Blocked,
		AnswerInterrupted: record.Cancelled,
	}
	for answer, want := range cases {
		if got := answer.Outcome(); got != want {
			t.Errorf("Answer(%d).Outcome() = %s, want %s", answer, got, want)
		}
	}
}
