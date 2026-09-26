// Package lifecycle owns the pure decision of what state a Task settles
// into, and the sole state cell a Task's outcome lives in. State's fields
// are unexported, so no code outside this package can assign them — the
// only way in is Declared, StartRunning and Settle, and Settle is the one
// place a target actually becomes the stored outcome: it runs every
// target through Decide itself, and it is where "terminal is final" is
// enforced, refusing a non-terminal target and refusing to move a State
// that has already settled. A caller-side Current()==Pending check before
// StartRunning, or a caller-side Decide call before Settle, is redundant:
// both methods already refuse the moves those checks were guarding
// against.
package lifecycle

import "github.com/zachbornheimer/evident-output/internal/core"

// DeclaresSuccess reports whether target claims the work went well — the
// class of claim only the scheduler's observation can ratify.
func DeclaresSuccess(target core.EntityState) bool {
	return target == core.Done || target == core.Skipped
}

// Decide is the one rule between a Task's blocking Problems and its
// terminal state: a Task holding any Problem cannot settle success-class,
// so a Done or Skipped claim over one settles Failed. It is pure — same
// inputs, same output, no engine types — so the matrix in
// lifecycle_test.go pins it directly. Settle calls it for every
// transition it applies; it is exported so a caller that needs to preview
// the outcome before Settle runs (e.g. to build a Problem's capture tail)
// can compute the same value Settle will store.
func Decide(target core.EntityState, hasProblems bool) core.EntityState {
	if DeclaresSuccess(target) && hasProblems {
		return core.Failed
	}
	return target
}

// State is the sole cell a Task's lifecycle state lives in.
type State struct {
	current core.EntityState
	settled bool
}

// Declared returns a State seeded at core.Pending. The zero State is
// equally valid: Current reports core.Pending for it too, so there is no
// separate "unset" value a caller could observe or branch on — Declared
// exists only to make a fresh Task's construction site read as a
// declaration rather than a zero value.
func Declared() State {
	return State{current: core.Pending}
}

// Current returns the state's present value. An unset State (the zero
// value) reads as core.Pending, the same value Declared seeds.
func (s State) Current() core.EntityState {
	if s.current == "" {
		return core.Pending
	}
	return s.current
}

// StartRunning moves the state to Running — the transition
// promoteRunningLocked applies on a Task's first unit of evidence. It
// refuses unless the state is still Pending, so a caller no longer needs
// its own Current()==Pending guard before calling in: a State already
// Running, or already settled (including Incomplete, which core.
// IsTerminalTask does not count as terminal), stays exactly where it was.
// ok reports whether the move happened; from is the state's value
// immediately before, valid only when ok is true.
func (s *State) StartRunning() (from core.EntityState, ok bool) {
	if s.Current() != core.Pending {
		return s.Current(), false
	}
	from = s.Current()
	s.current = core.Running
	return from, true
}

// Settle decides target against hasProblems (via Decide) and moves the
// state to the result — the one place a Task's terminal outcome is both
// decided and written, so "only lifecycle sets a Task's outcome" holds
// for the decision as well as the storage. It is also where "terminal is
// final" is enforced: it refuses Pending or Running (the two values only
// Declared/StartRunning may produce) as a target, and refuses to move a
// State that has already settled once, rather than let a second
// resolution silently overwrite the first. ok reports whether the
// transition happened. On success, resolved is the Decide'd value that
// was stored and from is the value the state held immediately before
// (callers use it for census bookkeeping, e.g. taskState.censusMoved). On
// rejection, resolved and from both hold the state's unchanged current
// value.
//
// Settle deliberately does not reuse core.IsTerminalTask to validate
// target: that function answers a different question (which states a
// *conclusion* treats as finished) and excludes Incomplete on purpose —
// yet Incomplete is a legitimate, one-time Settle target
// (settleUnresolvedTasksLocked). "Has this State settled before" is
// tracked here directly instead.
func (s *State) Settle(target core.EntityState, hasProblems bool) (resolved, from core.EntityState, ok bool) {
	if target == core.Pending || target == core.Running || s.settled {
		return s.Current(), s.Current(), false
	}
	decided := Decide(target, hasProblems)
	from = s.Current()
	s.current = decided
	s.settled = true
	return decided, from, true
}
