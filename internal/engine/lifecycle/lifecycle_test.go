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

func TestStateSettleAndStartRunning(t *testing.T) {
	s := Declared()
	if got := s.Current(); got != core.Pending {
		t.Fatalf("Current() after Declared() = %s, want pending", got)
	}
	from := s.StartRunning()
	if from != core.Pending {
		t.Fatalf("StartRunning from = %s, want pending", from)
	}
	if s.Current() != core.Running {
		t.Fatalf("Current() after StartRunning = %s, want running", s.Current())
	}
	resolved, from := s.Settle(core.Done, true)
	if from != core.Running {
		t.Fatalf("Settle from = %s, want running", from)
	}
	if resolved != core.Failed || s.Current() != core.Failed {
		t.Fatalf("Settle(Done, true) resolved = %s, current = %s, want failed", resolved, s.Current())
	}
}

// TestSyntheticConstructorsSeedTerminal pins that the only constructors
// producing a terminal State (the synthetic Output.Fail/Cancel path) land
// on the value their name promises, through Decide.
func TestSyntheticConstructorsSeedTerminal(t *testing.T) {
	if got := SettledFailed().Current(); got != core.Failed {
		t.Fatalf("SettledFailed().Current() = %s, want failed", got)
	}
	if got := SettledCancelled().Current(); got != core.Cancelled {
		t.Fatalf("SettledCancelled().Current() = %s, want cancelled", got)
	}
}
