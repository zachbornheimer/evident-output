package engine

import (
	"cmp"
	"math"
	"slices"
)

// collectionTally tracks a Group/Sequence's descendant Tasks for the
// Tasks that start after it. declareTaskLocked and settleLocked keep it
// current, so reading an outcome costs the same however large the
// collection grows.
//
// A predecessor edge on a collection reads only the members declared
// through its cursor (predecessor.through): naming a populated collection
// takes its membership as declared, so a member declared later never
// gates that edge, whatever the members' durations (E-092, E-093).
type collectionTally struct {
	// members is every descendant Task, in declaration order.
	members []*taskState
	// succeededPrefix is how many leading members succeeded.
	succeededPrefix int
	// firstFailed is the declaration of the earliest member that can
	// never succeed, or 0 while none has failed.
	firstFailed int
	// sealed records that the membership of an edge declared while the
	// collection was empty is taken as declared, through sealedThrough:
	// a Wait asked for its outcome, the run proved nothing will declare
	// into it (see sealInputsLocked), or it was named while populated
	// (see closeMembershipLocked). Until then such an edge waits for
	// every member, because its caller may declare more.
	sealed        bool
	sealedThrough int
	// parked are the Tasks waiting on this collection, by the cursor
	// their edge reads, ascending; open are those whose edge reads an
	// unsealed membership.
	parked []parkedTask
	open   []*taskState
}

// parkedTask is a Task parked on a collection edge that reads members
// declared through cursor.
type parkedTask struct {
	st     *taskState
	cursor int
}

// throughAll is the cursor of an edge that reads every member.
const throughAll = math.MaxInt

func (t *collectionTally) total() int { return len(t.members) }

// declare counts a newly declared member.
func (t *collectionTally) declare(st *taskState) {
	t.members = append(t.members, st)
	t.settle(st)
}

// settle records that member st may have settled and returns the Tasks
// whose edges it resolved.
func (t *collectionTally) settle(st *taskState) (woken []*taskState) {
	switch stateOutcome(st.state.Current()) {
	case predFailed:
		if t.firstFailed != 0 && t.firstFailed <= st.declaration {
			return nil
		}
		t.firstFailed = st.declaration
		woken = append(woken, t.open...)
		t.open = nil
		i := len(t.parked)
		for i > 0 && t.parked[i-1].cursor >= st.declaration {
			i--
		}
		for _, p := range t.parked[i:] {
			woken = append(woken, p.st)
		}
		t.parked = t.parked[:i]
		return woken
	case predSucceeded:
		for t.succeededPrefix < len(t.members) && stateOutcome(t.members[t.succeededPrefix].state.Current()) == predSucceeded {
			t.succeededPrefix++
		}
		n := 0
		for n < len(t.parked) && t.succeededThrough(t.parked[n].cursor) {
			woken = append(woken, t.parked[n].st)
			n++
		}
		t.parked = t.parked[n:]
		return woken
	default:
		return nil
	}
}

// seal takes the membership of edges declared while the collection was
// empty as declared through cursor, and returns the Tasks parked on
// those edges so they re-read it.
func (t *collectionTally) seal(cursor int) (woken []*taskState) {
	if t.sealed {
		return nil
	}
	t.sealed, t.sealedThrough = true, cursor
	woken, t.open = t.open, nil
	return woken
}

// hasMemberThrough reports whether any member was declared through cursor.
func (t *collectionTally) hasMemberThrough(cursor int) bool {
	return len(t.members) > 0 && t.members[0].declaration <= cursor
}

// membersThrough is the members declared after cursor from, through
// cursor to, in declaration order.
func (t *collectionTally) membersThrough(from, to int) []*taskState {
	byDecl := func(m *taskState, cursor int) int { return cmp.Compare(m.declaration, cursor) }
	lo, _ := slices.BinarySearchFunc(t.members, from+1, byDecl)
	hi, _ := slices.BinarySearchFunc(t.members, to+1, byDecl)
	return t.members[lo:hi]
}

// failedThrough reports whether a member declared through cursor can
// never succeed.
func (t *collectionTally) failedThrough(cursor int) bool {
	return t.firstFailed != 0 && t.firstFailed <= cursor
}

// succeededThrough reports whether every member declared through cursor
// succeeded.
func (t *collectionTally) succeededThrough(cursor int) bool {
	return t.succeededPrefix == len(t.members) || t.members[t.succeededPrefix].declaration > cursor
}

// park parks st on an edge reading members through cursor; open parks it
// on an edge that reads an unsealed membership.
func (t *collectionTally) park(st *taskState, cursor int, open bool) {
	if open {
		t.open = append(t.open, st)
		return
	}
	t.parked = insertAfterOrder(t.parked, parkedTask{st: st, cursor: cursor}, func(p parkedTask) int { return p.cursor })
}

// unpark drops every parked Task: the drain re-places them all.
func (t *collectionTally) unpark() {
	t.parked, t.open = nil, nil
}
