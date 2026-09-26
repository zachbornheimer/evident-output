// Package lifecycle owns the pure decision of what state a Task settles
// into, and the sole state cell a Task's outcome lives in. Nothing outside
// this package can write a Task's terminal outcome: State's field is
// unexported, so any attempt to set it from engine code other than through
// Settle or StartRunning is a compile error, not a convention.
package lifecycle

import "github.com/zachbornheimer/evident-output/internal/core"

// DeclaresSuccess reports whether target claims the work went well — the
// class of claim only the scheduler's observation can ratify (mirrors
// engine.declaresSuccess; kept here so Decide needs no engine import).
func DeclaresSuccess(target core.EntityState) bool {
	return target == core.Done || target == core.Skipped
}

// Decide is the one rule between a Task's blocking Problems and its
// terminal state: a Task holding any Problem cannot settle success-class,
// so a Done or Skipped claim over one settles Failed. It is pure — same
// inputs, same output, no engine types — so the matrix in
// lifecycle_test.go pins it directly.
func Decide(target core.EntityState, hasProblems bool) core.EntityState {
	if DeclaresSuccess(target) && hasProblems {
		return core.Failed
	}
	return target
}

// State is the sole cell a Task's lifecycle state lives in. Its field is
// unexported: no code outside this package can assign it, so "only
// lifecycle sets a Task's outcome" is enforced by the compiler, not by
// convention. The engine still owns locking — every method here assumes
// the caller already holds whatever lock guards the surrounding taskState,
// the same contract settleLocked/promoteRunningLocked documented before
// this extraction.
type State struct {
	current core.EntityState
}

// NewState returns a State starting at current — used once, when a
// taskState is constructed, to seed it at its initial value (typically
// core.NotStarted).
func NewState(current core.EntityState) State {
	return State{current: current}
}

// Current returns the state's present value.
func (s State) Current() core.EntityState {
	return s.current
}

// Settle applies Decide against hasProblems and moves the state to the
// resulting terminal value, returning both the resolved value and the
// value the state held immediately before (from) — callers use from for
// census bookkeeping (e.g. taskState.censusMoved) the way settleLocked
// always has.
func (s *State) Settle(target core.EntityState, hasProblems bool) (resolved, from core.EntityState) {
	from = s.current
	resolved = Decide(target, hasProblems)
	s.current = resolved
	return resolved, from
}

// StartRunning moves the state to Running, returning the value it held
// immediately before — the transition promoteRunningLocked applies on a
// Task's first unit of evidence. It is unconditional, the same as the
// direct assignment it replaces; callers already guard on Current() ==
// Pending before calling it.
func (s *State) StartRunning() (from core.EntityState) {
	from = s.current
	s.current = core.Running
	return from
}
