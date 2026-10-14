package graph

import (
	"fmt"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// EntityKind names the three declarable entity kinds §3.1's default stable
// key is built from (kind + parent stable key + normalized name). It is
// spelled out here, not reused from any presentation type, because identity
// and presentation are different concerns even though today they use the
// same words.
type EntityKind string

const (
	KindTask     EntityKind = "task"
	KindGroup    EntityKind = "group"
	KindSequence EntityKind = "sequence"
)

// ContainerKind is the kind a Group (sequential false) or Sequence declares.
func ContainerKind(sequential bool) EntityKind {
	if sequential {
		return KindSequence
	}
	return KindGroup
}

// DeclaredName is the single normalization every Task/Group/Sequence
// declaration path must apply to a caller-supplied name before using it for
// anything identity-related — sibling-dedup lookups and StableKey's own
// name segment alike. Two call sites normalizing separately (or one
// normalizing and one keying off the raw string) can disagree: a raw name
// carrying a control character evident-output strips on presentation would
// then dedup-check under a different string than the one its stable key
// derives from, letting "deploy\x01" and "deploy\x02" both register as
// distinct siblings of the same rendered name "deploy".
func DeclaredName(name string) string {
	return record.SanitizeText(name)
}

// StableKey computes §3.1's default identity: entity kind + parent stable
// key + normalized entity name. The application/workspace manifest
// namespace already supplies application identity, so it is not duplicated
// into every key here. name must already be DeclaredName-normalized.
func StableKey(kind EntityKind, parentKey, name string) string {
	return fmt.Sprintf("%s:%s/%s", kind, parentKey, name)
}

// Task is one declared operation as the scheduler knows it: who it is and
// where it sits. What it reported and found lives in Rec; how it is painted
// belongs to the projection that keys its own table by ID.
type Task struct {
	graph *Graph
	// ID names the Task in the record and in every projection's table.
	ID string
	// Name is the declared, normalized name.
	Name string
	// Declaration orders the Task among every declaration of the run.
	Declaration int
	// Rec is the Task's record: its state, phase, Problems and the rest.
	Rec *record.Task
	// Parent is the container the Task was declared under, nil at the root.
	Parent *Container

	key string // guarded by graph.mu

	// The rest is scheduling state, guarded by graph.mu.

	sched schedule
	// after is every edge After declared on this Task. sched.preds forgets
	// a satisfied Task predecessor; this keeps it, so Computed.Get can tell
	// an ordered reader from an unordered one.
	after []Predecessor
	// gateFor is set on a container builder's gate: the scheduler's entity
	// for the container's deferred declaration work. It is no row and in no
	// collection. Fixed at creation.
	gateFor *Container
	// proposal holds a caller's unratified success claim on a submitted
	// Task until the callback's return value confirms or contradicts it. It
	// is the Task's own, swapped rather than guarded by the graph's lock:
	// every callback reads it as it returns.
	proposal atomic.Pointer[Proposal]
	// readRefusal is the first Computed read refused to a goroutine the
	// Task's callback started. That goroutine cannot unwind the callback, so
	// the Task fails with it when the callback returns (see Reader).
	readRefusal refusal
	// overturn is the error another Task's act kept against this one before
	// it settled Done (see Overturn). It turns that Done into the Failure.
	overturn refusal
}

// Key is the Task's §3.1 stable key; see Graph.Rekey.
func (t *Task) Key() string {
	t.graph.mu.Lock()
	defer t.graph.mu.Unlock()
	return t.key
}

// Container is one declared Group or Sequence as the scheduler knows it.
// Its verdict derives from its members; Rec holds only what it said itself.
type Container struct {
	graph *Graph
	// ID names the container in the record and in every projection's table.
	ID string
	// Name is the declared, normalized name.
	Name string
	// Declaration orders the container among every declaration of the run.
	Declaration int
	// Sequential marks a Sequence: children run in declaration order.
	Sequential bool
	// Parent is the container this one is nested in, nil at the root.
	Parent *Container
	// Rec is the container's record.
	Rec *record.Container

	key   string // fixed at declaration
	path  record.ContainerPath
	names siblings
	// runningSteps are the steps of a Sequence that may still be Running
	// (StepStarted); its callers serialize access.
	runningSteps []*Task

	// The rest is scheduling state, guarded by graph.mu.

	// tasks and children are the Tasks and nested containers declared
	// directly in this container, in declaration order.
	tasks    []*Task
	children []*Container
	// entry is what everything declared in this container starts after:
	// the step before it when it is a step of a Sequence.
	entry []Predecessor
	// lastStep is, for a Sequence, what its next step starts after: the
	// one step declared most recently, Task or nested collection.
	lastStep []Predecessor
	// stoppedAfter is, for a Sequence, the declaration of the earliest step
	// a failure already stopped its later steps after (0: none yet).
	stoppedAfter int
	// tally counts this container's descendant Tasks by outcome, for the
	// Tasks that run After it.
	tally tally
	// builder is the deferred declaration work Define gave this container;
	// nil for one whose children are declared directly.
	builder *builder
}

// Key is the container's §3.1 stable key.
func (c *Container) Key() string { return c.key }

// Path is c, then every container above it, nearest first. It is fixed at
// declaration.
func (c *Container) Path() record.ContainerPath { return c.path }

// TaskID is the Task's id in the run record.
func (t *Task) TaskID() record.TaskID { return record.TaskID(t.ID) }

// ManifestKey is the Task's stable key in the manifest; with TaskID it makes
// a Task the view freshness judges it through.
func (t *Task) ManifestKey() string { return t.Key() }

// LedgerOwner is the Task as the ledger knows it: placed by declaration,
// found by id, shown by name, qualified by its containers.
func (t *Task) LedgerOwner() record.LedgerOwner {
	var containers record.ContainerPath
	if t.Parent != nil {
		containers = t.Parent.Path()
	}
	return record.LedgerOwner{ID: t.ID, Name: t.Name, Declaration: t.Declaration, Containers: containers}
}

// KeyOf is c's stable key, or "" for the root (nil).
func KeyOf(c *Container) string {
	if c == nil {
		return ""
	}
	return c.key
}
