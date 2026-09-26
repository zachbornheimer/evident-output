// Package lifecycle owns the pure decision of what state a Task settles
// into, and the sole state cell a Task's outcome lives in. Nothing outside
// this package can write a Task's terminal outcome: State's field is
// unexported, and the only constructors that produce a terminal State
// (SettledFailed, SettledCancelled) go through Decide — there is no
// general-purpose constructor that accepts an arbitrary EntityState, so a
// stray `state: lifecycle.NewState(core.Done)` seed can no longer compile
// its way past this package's rule.
package lifecycle

import "github.com/zachbornheimer/evident-output/internal/core"

// DeclaresSuccess reports whether target claims the work went well — the
// class of claim only the scheduler's observation can ratify. It is the
// one place this rule is named: engine no longer keeps its own copy, and
// asks this function directly wherever it used to.
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

// Declared returns a State seeded at core.Pending — the value a taskState
// starts at when a caller declares a new Task. It is the only constructor
// that seeds a non-terminal State; there is no general-purpose
// NewState(EntityState) escape hatch, so engine code cannot seed a Task
// straight into a terminal state without going through Settle.
func Declared() State {
	return State{current: core.Pending}
}

// SettledFailed returns a State already settled Failed, through Decide,
// for the synthetic failed Task Output.Fail creates — a Task that never
// ran and so never has a Running/Pending phase of its own.
func SettledFailed() State {
	var s State
	s.Settle(core.Failed, true)
	return s
}

// SettledCancelled returns a State already settled Cancelled, through
// Decide, for the synthetic cancelled Task Output.Cancel creates.
func SettledCancelled() State {
	var s State
	s.Settle(core.Cancelled, false)
	return s
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
