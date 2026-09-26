package lifecycle

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// TestDecide pins the pure outcome rule this package extracted from
// taskState.resolve (now lifecycle.Decide directly): a success-class target over a Task holding a
// Problem settles Failed, every other target passes through unchanged.
func TestDecide(t *testing.T) {
	cases := []struct {
		name        string
		target      core.EntityState
		hasProblems bool
		want        core.EntityState
	}{
		{"done, no problems", core.Done, false, core.Done},
		{"done, with problems", core.Done, true, core.Failed},
		{"skipped, no problems", core.Skipped, false, core.Skipped},
		{"skipped, with problems", core.Skipped, true, core.Failed},
		{"failed, no problems", core.Failed, false, core.Failed},
		{"failed, with problems", core.Failed, true, core.Failed},
		{"blocked, no problems", core.Blocked, false, core.Blocked},
		{"blocked, with problems", core.Blocked, true, core.Blocked},
		{"cancelled, with problems", core.Cancelled, true, core.Cancelled},
		{"not started, with problems", core.NotStarted, true, core.NotStarted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Decide(tc.target, tc.hasProblems); got != tc.want {
				t.Fatalf("Decide(%s, %v) = %s, want %s", tc.target, tc.hasProblems, got, tc.want)
			}
		})
	}
}

func TestZeroStateIsPending(t *testing.T) {
	var s State
	if got := s.Current(); got != core.Pending {
		t.Fatalf("zero State Current() = %s, want pending", got)
	}
	from, ok := s.StartRunning()
	if !ok || from != core.Pending {
		t.Fatalf("StartRunning() on zero State = (%s, %v), want (pending, true)", from, ok)
	}
}

func TestStateSettleAndStartRunning(t *testing.T) {
	s := Declared()
	if got := s.Current(); got != core.Pending {
		t.Fatalf("Current() after Declared() = %s, want pending", got)
	}
	from, ok := s.StartRunning()
	if !ok {
		t.Fatalf("StartRunning() ok = false, want true")
	}
	if from != core.Pending {
		t.Fatalf("StartRunning from = %s, want pending", from)
	}
	if s.Current() != core.Running {
		t.Fatalf("Current() after StartRunning = %s, want running", s.Current())
	}
	resolved, from, ok := s.Settle(core.Done, true)
	if !ok {
		t.Fatalf("Settle(Done, true) ok = false, want true")
	}
	if from != core.Running {
		t.Fatalf("Settle from = %s, want running", from)
	}
	if resolved != core.Failed || s.Current() != core.Failed {
		t.Fatalf("Settle(Done, true) resolved = %s, current = %s, want failed", resolved, s.Current())
	}
}

// TestStartRunningRefusesNonPending pins the blocking gap ZYS-1190 review
// found: StartRunning used to move current to Running unconditionally, so
// a Task already settled (including Incomplete, which core.IsTerminalTask
// does not treat as terminal) could be shoved back into Running and left
// stuck there forever. StartRunning now only moves a State out of
// Pending.
func TestStartRunningRefusesNonPending(t *testing.T) {
	t.Run("already running", func(t *testing.T) {
		s := Declared()
		s.StartRunning()
		if _, ok := s.StartRunning(); ok {
			t.Fatalf("StartRunning() on an already-Running State ok = true, want false")
		}
		if s.Current() != core.Running {
			t.Fatalf("Current() = %s, want running unchanged", s.Current())
		}
	})

	t.Run("settled incomplete", func(t *testing.T) {
		s := Declared()
		s.StartRunning()
		if _, _, ok := s.Settle(core.Incomplete, false); !ok {
			t.Fatalf("Settle(Incomplete) ok = false, want true")
		}
		if _, ok := s.StartRunning(); ok {
			t.Fatalf("StartRunning() after Settle(Incomplete) ok = true, want false — a settled Task must never re-enter Running")
		}
		if s.Current() != core.Incomplete {
			t.Fatalf("Current() = %s, want incomplete unchanged", s.Current())
		}
	})

	t.Run("settled done", func(t *testing.T) {
		s := Declared()
		s.StartRunning()
		s.Settle(core.Done, false)
		if _, ok := s.StartRunning(); ok {
			t.Fatalf("StartRunning() after Settle(Done) ok = true, want false")
		}
	})
}

// TestSettleRejectsNonTerminalTarget pins that Settle refuses a target
// that is not itself a terminal EntityState — Settle moves a Task to its
// outcome, it never parks it mid-flight.
func TestSettleRejectsNonTerminalTarget(t *testing.T) {
	s := Declared()
	s.StartRunning()
	resolved, _, ok := s.Settle(core.Running, false)
	if ok {
		t.Fatalf("Settle(Running) ok = true, want false")
	}
	if resolved != core.Running || s.Current() != core.Running {
		t.Fatalf("Settle(Running) left current = %s, want running unchanged", s.Current())
	}
}

// TestSettleRejectsAlreadyTerminal pins "terminal is final": once a State
// has settled, a second Settle call — from any target — is refused rather
// than silently overwriting the first outcome. This is what lets
// task_resolve.go rely on Settle's own ok result instead of checking
// core.IsTerminalTask itself before calling in.
func TestSettleRejectsAlreadyTerminal(t *testing.T) {
	s := Declared()
	s.StartRunning()
	if _, _, ok := s.Settle(core.Done, false); !ok {
		t.Fatalf("first Settle(Done) ok = false, want true")
	}
	resolved, from, ok := s.Settle(core.Cancelled, false)
	if ok {
		t.Fatalf("second Settle(Cancelled) ok = true, want false")
	}
	if resolved != core.Done || from != core.Done {
		t.Fatalf("rejected Settle reported resolved=%s from=%s, want both done (state must not move)", resolved, from)
	}
	if s.Current() != core.Done {
		t.Fatalf("Current() after rejected re-settle = %s, want done (first outcome must stick)", s.Current())
	}
}

// TestSettleAppliesDecide pins that Settle itself runs Decide against
// hasProblems — a caller can no longer settle a Task Done/Skipped over a
// Problem by forgetting to call Decide first, because Settle is the only
// way to write the state at all.
func TestSettleAppliesDecide(t *testing.T) {
	s := Declared()
	s.StartRunning()
	resolved, _, ok := s.Settle(core.Skipped, true)
	if !ok {
		t.Fatalf("Settle(Skipped, true) ok = false, want true")
	}
	if resolved != core.Failed || s.Current() != core.Failed {
		t.Fatalf("Settle(Skipped, true) resolved = %s, current = %s, want failed", resolved, s.Current())
	}
}
