package engine

import (
	"sync"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/manifest"
	"github.com/zachbornheimer/evident-output/internal/record"
)

type taskState struct {
	id   string
	key  string // optional stable machine key (platform ID)
	name string
	// rec is what this Task reported and found: its state, phase, progress,
	// summary, Problems, warnings, facts, verification, resolution and
	// skip and keep records. Everything that writes it goes through rec.
	rec         *record.Task
	actions     []Action
	collection  *tasksState
	declaration int
	handle      *TaskHandle
	// sched is where this Task stands with the scheduler.
	sched taskSchedule
	// gateFor is set on a container builder's gate: the scheduler's entity
	// for the container's deferred declaration work. It is no row and in no
	// collection (see containerBuilder).
	gateFor *tasksState
	// after is every edge After declared on this Task. sched.preds forgets
	// a satisfied Task predecessor; this keeps it, so Computed.Get can tell
	// an ordered reader from an unordered one.
	after []predecessor

	// activityAt is the domain-clock time of the most recent Phase, Progress,
	// or work-callback-starting call — kept for the public
	// ActivityAt snapshot field and for Sequence's "one Running child"
	// bookkeeping. P5's elapsed-time render clock no longer reads it (see
	// elapsedAfter in live.go): the render anchor is liveFirstSeenAt alone,
	// so a fresh Phase/Progress call never restarts the elapsed suffix.
	activityAt time.Time

	// liveFirstSeenAt is the domain-clock time this task was first actually
	// painted in the live region (see taskState.stampLiveFirstSeen in live.go) —
	// the one elapsed-time anchor every row's heartbeat suffix reads (P5),
	// including a Pending task, which never calls Phase/Progress.
	liveFirstSeenAt time.Time

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
	// proposed holds a caller's unratified success claim on a submitted task
	// until the callback's return value confirms or contradicts it.
	proposed *proposedOutcome
	doneOnce sync.Once
	doneCh   chan struct{}

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
	// key is the §3.1 stable machine identity for this Group/Sequence: the
	// default kind+parent-key+normalized-name derivation, computed once at
	// declaration (see declareContainerLocked).
	key         string
	name        string
	summary     string
	tasks       []*taskState
	declaration int
	handle      *GroupHandle

	// kids is what a live frame and the verdict read of tasks.
	kids childIndex

	// names holds the names this container's child Tasks and containers
	// claimed (§3.1); see siblings.
	names siblings

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
	// entry is what everything declared in this container starts after:
	// the step before it when it is a step of a Sequence (see
	// nextStepPreds).
	entry []predecessor
	// lastStep is, for a Sequence, what its next step starts after: the
	// one step declared most recently, Task or nested collection.
	lastStep []predecessor
	// stoppedAfter is, for a Sequence, the declaration of the earliest step
	// a failure already stopped its later steps after (0: none yet).
	stoppedAfter int
	// tally counts this container's descendant Tasks by outcome, for the
	// Tasks that run After it.
	tally collectionTally
	// census counts its descendant Tasks for the live frame (liveCensus).
	census liveCensus
	// builder is the deferred declaration work Define gave this container;
	// nil for one whose children are declared directly.
	builder *containerBuilder
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
