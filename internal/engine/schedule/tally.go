package schedule

import (
	"cmp"
	"math"
	"slices"

	"github.com/zachbornheimer/evident-output/internal/ordered"
)

// Member is what the queue and tallies read from a Task.
type Member interface {
	Declaration() int
	Outcome() Outcome
}

// ThroughAll is the cursor of an edge that reads every member.
const ThroughAll = math.MaxInt

// parked is a Task parked on a collection edge that reads members
// declared through cursor.
type parked[T Member] struct {
	st     T
	cursor int
}

// Tally tracks a collection's descendant Tasks for the Tasks that start
// after it. Declare and Settle keep it current, so reading an outcome
// costs the same however large the collection grows.
//
// A predecessor edge on a collection reads only the members declared
// through its cursor: naming a populated collection takes its membership
// as declared, so a member declared later never gates that edge, whatever
// the members' durations (E-092, E-093).
type Tally[T Member] struct {
	// members is every descendant Task, in declaration order.
	members []T
	// succeededPrefix is how many leading members succeeded.
	succeededPrefix int
	// firstFailed is the declaration of the earliest member that can
	// never succeed, or 0 while none has failed.
	firstFailed int
	// sealed records that the membership of an edge declared while the
	// collection was empty is taken as declared, through sealedThrough.
	// Until then such an edge waits for every member, because its caller
	// may declare more.
	sealed        bool
	sealedThrough int
	// parked are the Tasks waiting on this collection, by the cursor
	// their edge reads, ascending; open are those whose edge reads an
	// unsealed membership.
	parked []parked[T]
	open   []T
}

// Len is the number of declared members.
func (t *Tally[T]) Len() int { return len(t.members) }

// Declare counts a newly declared member.
func (t *Tally[T]) Declare(m T) {
	t.members = append(t.members, m)
	t.Settle(m)
}

// Settle records that member m may have settled and returns the Tasks
// whose edges it resolved.
func (t *Tally[T]) Settle(m T) (woken []T) {
	switch m.Outcome() {
	case Failed:
		if t.firstFailed != 0 && t.firstFailed <= m.Declaration() {
			return nil
		}
		t.firstFailed = m.Declaration()
		woken = append(woken, t.open...)
		t.open = nil
		i := len(t.parked)
		for i > 0 && t.parked[i-1].cursor >= m.Declaration() {
			i--
		}
		for _, p := range t.parked[i:] {
			woken = append(woken, p.st)
		}
		t.parked = t.parked[:i]
		return woken
	case Succeeded:
		for t.succeededPrefix < len(t.members) && t.members[t.succeededPrefix].Outcome() == Succeeded {
			t.succeededPrefix++
		}
		n := 0
		for n < len(t.parked) && t.SucceededThrough(t.parked[n].cursor) {
			woken = append(woken, t.parked[n].st)
			n++
		}
		t.parked = t.parked[n:]
		return woken
	default:
		return nil
	}
}

// Seal takes the membership of edges declared while the collection was
// empty as declared through cursor, and returns the Tasks parked on those
// edges so they re-read it.
func (t *Tally[T]) Seal(cursor int) (woken []T) {
	if t.sealed {
		return nil
	}
	t.sealed, t.sealedThrough = true, cursor
	woken, t.open = t.open, nil
	return woken
}

// SealedThrough returns the raw sealed cursor, and whether the tally is
// sealed.
func (t *Tally[T]) SealedThrough() (cursor int, sealed bool) {
	return t.sealedThrough, t.sealed
}

// AnyFailed reports whether any member has failed.
func (t *Tally[T]) AnyFailed() bool { return t.firstFailed != 0 }

// HasMemberThrough reports whether any member was declared through cursor.
func (t *Tally[T]) HasMemberThrough(cursor int) bool {
	return len(t.members) > 0 && t.members[0].Declaration() <= cursor
}

// MembersThrough is the members declared after cursor from, through
// cursor to, in declaration order.
func (t *Tally[T]) MembersThrough(from, to int) []T {
	byDecl := func(m T, cursor int) int { return cmp.Compare(m.Declaration(), cursor) }
	lo, _ := slices.BinarySearchFunc(t.members, from+1, byDecl)
	hi, _ := slices.BinarySearchFunc(t.members, to+1, byDecl)
	return t.members[lo:hi]
}

// FailedThrough reports whether a member declared through cursor can
// never succeed.
func (t *Tally[T]) FailedThrough(cursor int) bool {
	return t.firstFailed != 0 && t.firstFailed <= cursor
}

// SucceededThrough reports whether every member declared through cursor
// succeeded.
func (t *Tally[T]) SucceededThrough(cursor int) bool {
	return t.succeededPrefix == len(t.members) || t.members[t.succeededPrefix].Declaration() > cursor
}

// Park parks m on an edge reading members through cursor; open parks it on
// an edge that reads an unsealed membership.
func (t *Tally[T]) Park(m T, cursor int, open bool) {
	if open {
		t.open = append(t.open, m)
		return
	}
	t.parked = ordered.Insert(t.parked, parked[T]{st: m, cursor: cursor}, func(p parked[T]) int { return p.cursor })
}

// Unpark drops every parked Task: the drain re-places them all.
func (t *Tally[T]) Unpark() {
	t.parked, t.open = nil, nil
}
