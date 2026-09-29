package schedule

// Phase is where a Task stands with the scheduler. The phases are closed:
// every scheduling predicate reads one of them instead of re-deriving it
// from a conjunction of flags.
type Phase uint8

const (
	// Declared: the Task exists but was never Defined. Its verb is still
	// to come from the caller (or it resolved by a caller verb).
	Declared Phase = iota
	// Queued: Defined, every predecessor succeeded, waiting in the
	// scheduler queue for a slot.
	Queued
	// Parked: Defined, and waiting off the queue on the one predecessor
	// it is blocked by.
	Parked
	// Running: its callback was claimed by a worker or a waiter.
	Running
	// Abandoned: Defined, but settled NotStarted without ever running.
	Abandoned
)

// Standing is one Task's phase. Zero value is Declared. Only Board.Move
// writes it, so the parked count cannot drift.
type Standing struct{ phase Phase }

// Phase is s's current phase.
func (s Standing) Phase() Phase { return s.phase }

// Submitted reports whether Define submitted the Task's work and the
// scheduler has not abandoned it.
func (s Standing) Submitted() bool {
	return s.phase == Queued || s.phase == Parked || s.phase == Running
}

// AwaitingStart reports whether the work was submitted and nobody has
// started it yet. The caller still checks the Task is not terminal.
func (s Standing) AwaitingStart() bool {
	return s.phase == Queued || s.phase == Parked
}

// Board counts Parked standings.
type Board struct{ parked int }

// Move moves s to phase, keeping the parked count true.
func (b *Board) Move(s *Standing, phase Phase) {
	if s.phase == Parked {
		b.parked--
	}
	if phase == Parked {
		b.parked++
	}
	s.phase = phase
}

// Parked is the number of Standings currently Parked.
func (b *Board) Parked() int { return b.parked }
