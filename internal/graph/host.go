// This file declares what the graph's per-Task bookkeeping needs of the run
// around it, and of freshness. The root and freshness implement them; graph
// imports neither.

package graph

import (
	"context"
	"sync"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// Host is what the run around the graph lends each Task's callback and each
// resolution: its lock, its lifecycle, and the edge where the reader hears
// what happened.
type Host interface {
	// Lock and Unlock bracket a submission or a resolution under the run's
	// lock, which comes before graph's in the lock order.
	sync.Locker
	// CheckOpen fails, recording misuse, once the run is closing. The
	// caller holds the lock.
	CheckOpen() error
	// DryRun reports whether the run plans its changes instead of making
	// them. It takes the lock itself.
	DryRun() bool
	// Ending is the state and Problem a callback's returned error settles.
	Ending(err error) (record.State, record.Problem)
	TaskNarration
}

// TaskNarration is how the reader learns what happened to a Task's work.
type TaskNarration interface {
	// Submitted follows the record once work was submitted for t. The
	// caller holds the lock.
	Submitted(t *Task)
	// Started does the render work a claimed Task owes, and reports whether
	// it began: false when it settled between its claim and this step. It
	// takes the lock itself.
	Started(t *Task) bool
	// Resolving lets a running row finish its paint before it resolves. It
	// takes the lock itself.
	Resolving(t *Task)
	// AlreadyResolved records the misuse of resolving t a second time,
	// keeping the rejected summary. The caller holds the lock.
	AlreadyResolved(t *Task, summary string)
	// Disowned reports that t resolved itself, from inside its own
	// callback, as anything but a success: the work in flight for it
	// records nothing. The caller holds the lock.
	Disowned(t *Task)
	// Settled commits t's settled row where its readers find it. The caller
	// holds the lock.
	Settled(t *Task)
	// RunEnded registers the Task that carries the run's own ending. The
	// caller holds the lock.
	RunEnded(t *Task)
	// DefinitionStarted and DefinitionFinished bracket t's callback. They
	// take the lock themselves.
	DefinitionStarted(t *Task)
	DefinitionFinished(t *Task, failed bool)
	// EvidenceEvaluated reports one evidence phase recorded on t. It takes
	// the lock itself.
	EvidenceEvaluated(t *Task, phase EvidencePhase, evidence record.EvidencePhase)
}

// EvidencePhase is which of a Task's two observations (§30) evidence
// belongs to.
type EvidencePhase uint8

const (
	// EvidenceBefore is observed as the Task starts, before its callback.
	EvidenceBefore EvidencePhase = iota
	// EvidenceAfter is observed once its callback succeeded.
	EvidenceAfter
)

// Checks decide, around one Task's callback, whether its work is needed and
// whether it held.
type Checks interface {
	// Current reports, as the Task starts, whether its declared Basis says
	// the work is already done. A Task with no Basis is never current.
	Current(ctx context.Context) (bool, error)
	// Verify runs the Task's Verify checks, ANDed. evaluated is false when
	// it registered none.
	Verify(ctx context.Context) (evaluated, satisfied bool, err error)
	// Operations is how many tracked operations the callback ran, and
	// whether the manifest proved every one current.
	Operations() (count int, allCurrent bool)
}

// WithHost makes h the run around the graph. A graph built without one
// cannot run Define.
func WithHost(h Host) Option { return func(g *Graph) { g.host = h } }
