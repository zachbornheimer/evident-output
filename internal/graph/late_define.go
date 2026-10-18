// This file owns the one rule for a Define that comes too late: once the
// run body has returned, Finish drains the queue and no new work may join
// it, whatever the Task named has waiting after it (CONTRACT.md section 7).

package graph

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/v2/internal/misuse"
)

// lateDefineRefusal is the misuse a Define records when it arrives after the
// run body returned. Its detail is the corrective sentence the reader sees.
func lateDefineRefusal(name string) error {
	return fmt.Errorf("%w: Define %s before the run body returns, or declare it in a Build", misuse.ErrInvalidConfig, name)
}

// DefineIsLate reports whether a Define made by the calling goroutine now
// arrives after the run body returned: the drain began, and the caller is
// not a container builder, which declares the topology the drain itself
// reaches. The caller holds no lock the graph takes.
func (g *Graph) DefineIsLate() bool {
	return g.Draining() && !g.insideBuilder()
}

// RefuseLateDefine refuses a late Define of t, in every case: with a Task
// waiting After t or none, with t still pending or already ended NotStarted
// by the drain. It reports whether it refused; false means t already has
// work, so the Define is a second one and the ordinary rule judges it. The
// refused Task has no work and never will, so it ends NotStarted now, the
// state the drain already gave a Task that something waited on. The caller
// holds the run's lock and records lateDefineRefusal once it releases it.
func (g *Graph) RefuseLateDefine(t *Task) bool {
	g.lock()
	defer g.unlock()
	if !t.neverSubmittedLocked() {
		return false
	}
	g.enterPhaseLocked(t, PhaseRefused)
	if !terminal(t) {
		g.markNotStartedLocked(t)
	}
	return true
}

// insideBuilder reports whether the calling goroutine is running a container
// builder and no Task callback beyond it.
func (g *Graph) insideBuilder() bool {
	if g.Executing() == 0 {
		return false
	}
	m := ReadStackMarks()
	return m.Builders > 0 && m.Callbacks <= m.Builders
}

// neverSubmittedLocked reports whether no Define has ever submitted work for
// t: it was never Defined, or an earlier Define was refused as late.
func (t *Task) neverSubmittedLocked() bool {
	return t.sched.phase == PhaseDeclared || t.sched.phase == PhaseRefused
}
