package graph

import (
	"fmt"

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
	names siblings
}

// Key is the container's §3.1 stable key.
func (c *Container) Key() string { return c.key }

// KeyOf is c's stable key, or "" for the root (nil).
func KeyOf(c *Container) string {
	if c == nil {
		return ""
	}
	return c.key
}
