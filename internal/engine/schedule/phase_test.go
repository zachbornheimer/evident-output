package schedule

import "testing"

// TestBoardParkedTracksMoves walks a Standing through
// Declared→Queued→Parked→Queued→Running and then a second Standing through
// Parked→Abandoned, checking Board.Parked() after every move.
func TestBoardParkedTracksMoves(t *testing.T) {
	var b Board
	var s Standing

	steps := []struct {
		to     Phase
		parked int
	}{
		{Queued, 0},
		{Parked, 1},
		{Queued, 0},
		{Running, 0},
	}
	for _, step := range steps {
		b.Move(&s, step.to)
		if s.Phase() != step.to {
			t.Fatalf("after Move(%v): Phase() = %v, want %v", step.to, s.Phase(), step.to)
		}
		if got := b.Parked(); got != step.parked {
			t.Fatalf("after Move(%v): Parked() = %d, want %d", step.to, got, step.parked)
		}
	}

	var s2 Standing
	b.Move(&s2, Parked)
	if got := b.Parked(); got != 1 {
		t.Fatalf("after second Standing Parked: Parked() = %d, want 1", got)
	}
	b.Move(&s2, Abandoned)
	if got := b.Parked(); got != 0 {
		t.Fatalf("after Parked->Abandoned: Parked() = %d, want 0", got)
	}
}

// TestStandingZeroValueIsDeclared checks the zero value of Standing needs
// no construction.
func TestStandingZeroValueIsDeclared(t *testing.T) {
	var s Standing
	if s.Phase() != Declared {
		t.Fatalf("zero Standing Phase() = %v, want Declared", s.Phase())
	}
}

// TestStandingSubmittedAwaitingStart is the truth table for Submitted and
// AwaitingStart over every Phase.
func TestStandingSubmittedAwaitingStart(t *testing.T) {
	cases := []struct {
		phase         Phase
		submitted     bool
		awaitingStart bool
	}{
		{Declared, false, false},
		{Queued, true, true},
		{Parked, true, true},
		{Running, true, false},
		{Abandoned, false, false},
	}
	for _, c := range cases {
		s := Standing{phase: c.phase}
		if got := s.Submitted(); got != c.submitted {
			t.Errorf("Phase %v: Submitted() = %v, want %v", c.phase, got, c.submitted)
		}
		if got := s.AwaitingStart(); got != c.awaitingStart {
			t.Errorf("Phase %v: AwaitingStart() = %v, want %v", c.phase, got, c.awaitingStart)
		}
	}
}
