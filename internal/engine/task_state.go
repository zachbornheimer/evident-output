package engine

import (
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/manifest"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// key is the Task's §3.1 stable key, empty for a Task the graph never declared.
func (st *taskState) key() string { return st.node.Key() }

// awaitingStart reports whether st is submitted work nobody has started or
// resolved.
func (st *taskState) awaitingStart() bool { return st.node.AwaitingStart() }

// neverDefined reports whether st is still waiting on its caller: declared,
// never Defined, and not resolved by a verb either.
func (st *taskState) neverDefined() bool { return st.node.NeverDefined() }

// isGate reports whether st is a container builder's gate, no row and in no
// collection.
func (st *taskState) isGate() bool { return st.node.IsGate() }

// key is the container's §3.1 stable key.
func (g *tasksState) key() string { return g.node.Key() }

type taskState struct {
	id string
	// node is the graph's declaration of this Task. It owns the Task's stable
	// key and everything the scheduler knows: its phase, predecessors and
	// the work Define submitted.
	node *graph.Task
	name string
	// rec is what this Task reported and found: its state, phase, progress,
	// summary, Problems, warnings, facts, verification, resolution and
	// skip and keep records. Everything that writes it goes through rec.
	rec *record.Task
	// collection and declaration copy the node's Parent and Declaration, for
	// the render code that walks them without asking the graph.
	collection  *tasksState
	declaration int
	handle      *TaskHandle

	// liveFirstSeenAt is the domain-clock time this task was first actually
	// painted in the live region (see taskState.stampLiveFirstSeen in live.go) —
	// the one elapsed-time anchor every row's heartbeat suffix reads (P5),
	// including a Pending task, which never calls Phase/Progress.
	liveFirstSeenAt time.Time

	// followed is the state the render state last accounted for: the live
	// census counts the Task under it, and a later difference from the
	// record's state is what followRecordLocked reacts to. A Task starts
	// followed as declaredState, so a Task settled before the engine ever
	// saw it still gets its first reaction.
	followed EntityState
	// settleAnswered is set once the render work a settle owes was queued.
	settleAnswered bool

	// filing is where this Task stands in its collection's childIndex.
	filing filing

	// heartbeat is the §40 plain-mode durable heartbeat's state.
	heartbeat plainHeartbeat

	// capture is the get-or-create sink shared by Task.Capture and PhaseWriter
	// so child-process evidence recorded via either path lands in one ring and
	// DetailTail sees it after Fail.
	evidence *evidence
	// tail is the bounded window of Writer lines a live frame draws beneath
	// this row; evidence above stays the full record.
	tail liveTail

	// Emission bookkeeping so terminal standalone tasks stream in plain mode
	// on resolve (P2).
	coreEmitted bool

	// synthetic marks a task the library invented to carry an output-level
	// outcome (Output.Fail/Cancel's synthetic "command" task) rather than
	// one the caller declared. shouldSuppressRepeatedCondition (I2) must
	// never drop the standalone conclusion band for one of these: it is the
	// only place the run's outcome is ever stated, unlike a caller-declared
	// Task whose own row already says the same thing.
	synthetic bool

	// effectDenials counts the times this task's own mutation callback
	// resolved the row as something other than Done while an Effect ran, so
	// that Effect's work must not reach the ledger (see deniesItsOwnEffect).
	// Every in-flight Effect compares it at entry and exit.
	effectDenials int
	// effectsInFlight counts evo.Effect callbacks currently running for this
	// task; a non-Done resolution while one runs disowns that Effect.
	effectsInFlight int
	// verifiers holds TaskHandle.Verify's registered pre/post-Define
	// observation checks, ANDed in registration order (§9.1). Must be
	// registered before Define — see Verify.
	verifiers []verifierFunc
	// workErr is the callback's own return value, kept so TaskHandle.Wait
	// returns exactly what the work returned rather than a state guess.
	workErr error

	// plainStream is what plain progressive streaming already emitted for
	// this still-Running standalone task.
	plainStream plainStreamMark

	// manifestOps accumulates this Task's tracked operation records for the
	// current Run (spec §11.3-11.5): one entry per evo.File/evo.Exec call
	// that participated in manifest tracking, appended in call order (the
	// same order manifest.Store.Operation's ordinal indexes into). Never
	// populated during dry-run — dry-run commits nothing (§8.2).
	manifestOps []manifest.OperationRecord

	// basisInputs are the freshness inputs declared by TaskHandle.Basis,
	// frozen at Define. basisObserved is their identity as observed when
	// the Task started this Run; it is committed with the Task's record.
	basisInputs   []BasisSource
	basisObserved []manifest.BasisRecord
}

type tasksState struct {
	id string
	// out is the Output this container belongs to: a read of its live index
	// first follows the record (see Output.followRecordLocked).
	out *Output
	// node is the graph's declaration of this Group/Sequence and owns its
	// §3.1 stable key.
	node *graph.Container
	name string
	// rec is what this container said about itself: its summary. Its verdict
	// derives from its members.
	rec         *record.Container
	tasks       []*taskState
	declaration int
	handle      *GroupHandle

	// kids is what a live frame and the verdict read of tasks.
	kids childIndex

	// sequential marks a Sequence: children are chained in declaration
	// order. A Group's children are independent and may overlap.
	sequential bool
	// runningSteps holds the children promoteRunningLocked moved to
	// Running that may still be Running (pruned on each promotion), so the
	// "one Running child" check never rescans every step.
	runningSteps []*taskState

	// children holds nested containers declared via Group.Group,
	// Group.Sequence, Sequence.Group, or Sequence.Sequence (P3's recursive
	// nesting) — a container's derived state and
	// rendering fold its children in exactly the way it folds its own
	// tasks.
	children []*tasksState

	// parent is the container this one is nested in, nil at the root.
	parent *tasksState
	// path caches containerPath: a container's place in the tree is fixed at
	// declaration, and every child's ledger section shares the one slice.
	path core.ContainerPath
	// census counts its descendant Tasks for the live frame (liveCensus).
	census liveCensus
	// hasNamesake records that a child Task carries this container's own
	// name, the only way it can render as its own Task (liveOwnRow).
	hasNamesake bool
}

// containerPath is the chain of containers enclosing st, nearest first;
// nil for a root Task.
func (st *taskState) containerPath() core.ContainerPath {
	if st.collection == nil {
		return nil
	}
	return st.collection.containerPath()
}

// containerPath is c then every container above it.
func (c *tasksState) containerPath() core.ContainerPath {
	if c.path == nil {
		c.path = core.ContainerPath{{ID: c.id, Name: c.name}}
		if c.parent != nil {
			c.path = append(c.path, c.parent.containerPath()...)
		}
	}
	return c.path
}
