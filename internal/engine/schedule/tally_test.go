package schedule

import "testing"

type fakeMember struct {
	decl int
	out  Outcome
}

func (m fakeMember) Declaration() int { return m.decl }
func (m fakeMember) Outcome() Outcome { return m.out }

func TestTallySettleFailedWakesOpenAndLaterParked(t *testing.T) {
	var ta Tally[fakeMember]
	early := fakeMember{decl: 1, out: Pending}
	failing := fakeMember{decl: 2, out: Pending}
	ta.Declare(early)
	ta.Declare(failing)

	openWaiter := fakeMember{decl: 100}
	ta.Park(openWaiter, 0, true)
	earlierWaiter := fakeMember{decl: 101}
	ta.Park(earlierWaiter, 1, false) // cursor before the failing member: not woken
	laterWaiter := fakeMember{decl: 102}
	ta.Park(laterWaiter, 2, false) // cursor at/after the failing member: woken

	failing.out = Failed
	woken := ta.Settle(failing)

	found := map[int]bool{}
	for _, m := range woken {
		found[m.Declaration()] = true
	}
	if !found[100] {
		t.Errorf("open waiter not woken: %v", woken)
	}
	if !found[102] {
		t.Errorf("later parked waiter not woken: %v", woken)
	}
	if found[101] {
		t.Errorf("earlier parked waiter woken unexpectedly: %v", woken)
	}
}

func TestTallySealIdempotentReturnsOpenOnce(t *testing.T) {
	var ta Tally[fakeMember]
	waiter := fakeMember{decl: 1}
	ta.Park(waiter, 0, true)

	woken := ta.Seal(5)
	if len(woken) != 1 || woken[0].Declaration() != 1 {
		t.Fatalf("first Seal: got %v", woken)
	}
	cursor, sealed := ta.SealedThrough()
	if !sealed || cursor != 5 {
		t.Fatalf("SealedThrough() = %d, %v; want 5, true", cursor, sealed)
	}

	again := ta.Seal(9)
	if len(again) != 0 {
		t.Fatalf("second Seal returned %v, want none", again)
	}
	cursor, sealed = ta.SealedThrough()
	if !sealed || cursor != 5 {
		t.Fatalf("SealedThrough() after second Seal = %d, %v; want unchanged 5, true", cursor, sealed)
	}
}

func TestTallySucceededPrefixAdvancesOverRun(t *testing.T) {
	var ta Tally[fakeMember]
	m1 := fakeMember{decl: 1, out: Pending}
	m2 := fakeMember{decl: 2, out: Pending}
	m3 := fakeMember{decl: 3, out: Pending}
	ta.Declare(m1)
	ta.Declare(m2)
	ta.Declare(m3)

	m1.out = Succeeded
	ta.Settle(m1)
	if ta.SucceededThrough(1) {
		t.Fatalf("SucceededThrough(1) = true before m1 recorded")
	}

	// Re-declare members with updated outcomes to advance the prefix, since
	// Settle reads the outcome from the member it is given, not a shared
	// pointer.
	var ta2 Tally[*mutableMember]
	a := &mutableMember{decl: 1}
	b := &mutableMember{decl: 2}
	c := &mutableMember{decl: 3}
	ta2.Declare(a)
	ta2.Declare(b)
	ta2.Declare(c)

	a.out = Succeeded
	ta2.Settle(a)
	if !ta2.SucceededThrough(1) {
		t.Fatalf("SucceededThrough(1) = false after a succeeded")
	}
	if ta2.SucceededThrough(2) {
		t.Fatalf("SucceededThrough(2) = true before b succeeded")
	}

	b.out = Succeeded
	ta2.Settle(b)
	if !ta2.SucceededThrough(2) {
		t.Fatalf("SucceededThrough(2) = false after a and b succeeded")
	}
}

type mutableMember struct {
	decl int
	out  Outcome
}

func (m *mutableMember) Declaration() int { return m.decl }
func (m *mutableMember) Outcome() Outcome { return m.out }

func TestTallyMembersThroughBounds(t *testing.T) {
	var ta Tally[fakeMember]
	ta.Declare(fakeMember{decl: 1})
	ta.Declare(fakeMember{decl: 2})
	ta.Declare(fakeMember{decl: 3})
	ta.Declare(fakeMember{decl: 4})

	got := ta.MembersThrough(1, 3)
	if len(got) != 2 || got[0].Declaration() != 2 || got[1].Declaration() != 3 {
		t.Fatalf("MembersThrough(1, 3) = %v", got)
	}

	if got := ta.MembersThrough(0, 0); len(got) != 0 {
		t.Fatalf("MembersThrough(0, 0) = %v, want none", got)
	}

	if got := ta.MembersThrough(0, 4); len(got) != 4 {
		t.Fatalf("MembersThrough(0, 4) = %v, want all 4", got)
	}
}
