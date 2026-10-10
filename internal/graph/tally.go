package graph

import (
	"cmp"
	"math"
	"slices"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// tally tracks a Group/Sequence's descendant Tasks for the Tasks that start
// after it. AddTask and settle keep it current, so reading an outcome costs
// the same however large the collection grows.
//
// A predecessor edge on a collection reads only the members declared
// through its cursor (Predecessor.through): naming a populated collection
// takes its membership as declared, so a member declared later never gates
// that edge, whatever the members' durations (E-092, E-093).
type tally struct {
	// members is every descendant Task, in declaration order.
	members []*Task
	// succeededPrefix is how many leading members succeeded.
	succeededPrefix int
	// firstFailed is the declaration of the earliest member that can
	// never succeed, or 0 while none has failed.
	firstFailed int
	// sealed records that the membership of an edge declared while the
	// collection was empty is taken as declared, through sealedThrough:
	// a Wait asked for its outcome, the run proved nothing will declare
	// into it (see sealInputs), or it was named while populated (see
	// closeMembership). Until then such an edge waits for every member,
	// because its caller may declare more.
	sealed        bool
	sealedThrough int
	// parked are the Tasks waiting on this collection, by the cursor
	// their edge reads, ascending; open are those whose edge reads an
	// unsealed membership.
	parked []parkedTask
	open   []*Task
	// holds counts the topology builders still pending in this container
	// or beneath it. While any is, the membership is not final, so no edge
	// on it is sealed or closed.
	holds int
	// builderFailed records that a builder under this container never
	// declared its children, so the container can never fully succeed.
	builderFailed bool
}

// parkedTask is a Task parked on a collection edge that reads members
// declared through cursor.
type parkedTask struct {
	t      *Task
	cursor int
}

// throughAll is the cursor of an edge that reads every member.
const throughAll = math.MaxInt

func (t *tally) total() int { return len(t.members) }

// declare counts a newly declared member.
func (t *tally) declare(m *Task) {
	t.members = append(t.members, m)
	t.settle(m)
}

// settle records that member m may have settled and returns the Tasks whose
// edges it resolved.
func (t *tally) settle(m *Task) (woken []*Task) {
	switch stateOutcome(m.Rec.State()) {
	case predFailed:
		if t.firstFailed != 0 && t.firstFailed <= m.Declaration {
			return nil
		}
		t.firstFailed = m.Declaration
		woken = append(woken, t.open...)
		t.open = nil
		i := len(t.parked)
		for i > 0 && t.parked[i-1].cursor >= m.Declaration {
			i--
		}
		for _, p := range t.parked[i:] {
			woken = append(woken, p.t)
		}
		t.parked = t.parked[:i]
		return woken
	case predSucceeded:
		for t.succeededPrefix < len(t.members) && stateOutcome(t.members[t.succeededPrefix].Rec.State()) == predSucceeded {
			t.succeededPrefix++
		}
		n := 0
		for n < len(t.parked) && t.succeededThrough(t.parked[n].cursor) {
			woken = append(woken, t.parked[n].t)
			n++
		}
		t.parked = t.parked[n:]
		return woken
	default:
		return nil
	}
}

// seal takes the membership of edges declared while the collection was
// empty as declared through cursor, and returns the Tasks parked on those
// edges so they re-read it.
func (t *tally) seal(cursor int) (woken []*Task) {
	if t.sealed || t.holds > 0 {
		return nil
	}
	t.sealed, t.sealedThrough = true, cursor
	woken, t.open = t.open, nil
	return woken
}

// hasMemberThrough reports whether any member was declared through cursor.
func (t *tally) hasMemberThrough(cursor int) bool {
	return len(t.members) > 0 && t.members[0].Declaration <= cursor
}

// membersThrough is the members declared after cursor from, through cursor
// to, in declaration order.
func (t *tally) membersThrough(from, to int) []*Task {
	byDecl := func(m *Task, cursor int) int { return cmp.Compare(m.Declaration, cursor) }
	lo, _ := slices.BinarySearchFunc(t.members, from+1, byDecl)
	hi, _ := slices.BinarySearchFunc(t.members, to+1, byDecl)
	return t.members[lo:hi]
}

// failedThrough reports whether a member declared through cursor can never
// succeed.
func (t *tally) failedThrough(cursor int) bool {
	return t.firstFailed != 0 && t.firstFailed <= cursor
}

// succeededThrough reports whether every member declared through cursor
// succeeded.
func (t *tally) succeededThrough(cursor int) bool {
	return t.succeededPrefix == len(t.members) || t.members[t.succeededPrefix].Declaration > cursor
}

// park parks m on an edge reading members through cursor; open parks it on
// an edge that reads an unsealed membership.
func (t *tally) park(m *Task, cursor int, open bool) {
	if open {
		t.open = append(t.open, m)
		return
	}
	t.parked = record.InsertAfterOrder(t.parked, parkedTask{t: m, cursor: cursor}, func(p parkedTask) int { return p.cursor })
}

// unpark drops every parked Task: the drain re-places them all.
func (t *tally) unpark() {
	t.parked, t.open = nil, nil
}

// release drops one pending builder's hold. When the last hold goes, a
// container that ran the builder takes its membership as declared (seal);
// an ancestor stays open for its own caller and only re-reads. It returns
// the Tasks to place again.
func (t *tally) release(seal bool, cursor int) (woken []*Task) {
	t.holds--
	if t.holds > 0 {
		return nil
	}
	if seal {
		return t.seal(cursor)
	}
	woken, t.open = t.open, nil
	return woken
}

// failBuilder records that a builder under this container never declared its
// children, and returns every Task waiting on the container.
func (t *tally) failBuilder() (woken []*Task) {
	t.builderFailed = true
	woken, t.open = t.open, nil
	for _, p := range t.parked {
		woken = append(woken, p.t)
	}
	t.parked = nil
	return woken
}
