// This file owns what freshness remembers about each Task during a Run: the
// inputs it declared, the Basis it observed, and the operation records it will
// commit.

package freshness

import (
	"slices"
	"sync"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// TaskView is the part of a Task freshness reads. The caller's Task type
// implements it, so freshness neither imports that type nor adds fields to
// it: everything freshness remembers about a Task lives in a TaskTable keyed
// by the Task's record id.
type TaskView interface {
	// TaskID is the Task's id in the run record.
	TaskID() record.TaskID
	// ManifestKey is the Task's stable key (§3.1) in the manifest.
	ManifestKey() string
}

// taskEntry is what freshness remembers about one Task.
type taskEntry struct {
	// inputs are the freshness inputs declared by Basis, frozen at Define.
	inputs []BasisSource
	// observed is their identity as observed when the Task started this Run;
	// it is committed with the Task's record.
	observed []BasisRecord
	// operations accumulates the Task's tracked operation records for the
	// current Run (spec §11.3-11.5), in call order: the order
	// ManifestStore.Operation's ordinal indexes into. A planned Run fills it
	// as an applying one does, but never commits it (see TaskTable.Commit).
	operations []OperationRecord
	// planned counts the operations a planned Run found stale and did not
	// apply: they hold their place in the order but leave no record.
	planned int
	// verifiers are the Verify checks, ANDed in registration order (§9.1).
	verifiers []Verifier
}

// TaskTable remembers freshness state per Task, keyed by record.TaskID. It
// is safe for concurrent use.
type TaskTable struct {
	mu    sync.Mutex
	tasks map[record.TaskID]*taskEntry
	// planned is what the operations a planned Run left unapplied would
	// change.
	planned plannedPaths
}

// NewTaskTable returns an empty table.
func NewTaskTable() *TaskTable {
	return &TaskTable{tasks: make(map[record.TaskID]*taskEntry), planned: make(plannedPaths)}
}

// entryLocked is id's entry, created on first use. Callers hold t.mu.
func (t *TaskTable) entryLocked(id record.TaskID) *taskEntry {
	entry := t.tasks[id]
	if entry == nil {
		entry = &taskEntry{}
		t.tasks[id] = entry
	}
	return entry
}

// AddInputs declares freshness inputs for id. Calls accumulate; a nil input
// is ignored.
func (t *TaskTable) AddInputs(id record.TaskID, inputs ...BasisSource) {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := t.entryLocked(id)
	for _, in := range inputs {
		if in != nil {
			entry.inputs = append(entry.inputs, in)
		}
	}
}

// AppendOperation records rec as id's next pending operation, committed only
// if and when the Task itself settles Done (spec §8.2/§11.3).
func (t *TaskTable) AppendOperation(id record.TaskID, rec OperationRecord) {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := t.entryLocked(id)
	entry.operations = append(entry.operations, rec)
}

// PlanOperation notes that id had an operation a planned Run found stale and
// did not apply, which would change changes. It leaves no record, but the
// operation after it must be judged against its own record, as it would
// after an applied one, and work reading changes is uncertain (see Plan).
func (t *TaskTable) PlanOperation(id record.TaskID, changes ...string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entryLocked(id).planned++
	for _, path := range changes {
		t.planned.add(path)
	}
}

// Plan is the Plan of a Run that only plans its work, judged against what
// this table's planned operations would change.
func (t *TaskTable) Plan() Plan { return Plan{table: t} }

func (t *TaskTable) plansChange(basis []BasisRecord) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.planned.changesAny(basis)
}

// OperationCount is how many operations id has had: the ordinal its next
// operation takes within its manifest record.
func (t *TaskTable) OperationCount(id record.TaskID) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if entry := t.tasks[id]; entry != nil {
		return len(entry.operations) + entry.planned
	}
	return 0
}

// AllCurrent reports whether id had at least one operation and the manifest
// proved every one current, so the run executed and planned none of them.
func (t *TaskTable) AllCurrent(id record.TaskID) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := t.tasks[id]
	if entry == nil || entry.planned > 0 || len(entry.operations) == 0 {
		return false
	}
	for _, op := range entry.operations {
		if !op.Carried {
			return false
		}
	}
	return true
}

// Operations is a copy of id's pending operation records.
func (t *TaskTable) Operations(id record.TaskID) []OperationRecord {
	t.mu.Lock()
	defer t.mu.Unlock()
	if entry := t.tasks[id]; entry != nil {
		return slices.Clone(entry.operations)
	}
	return nil
}

// Tracked is what id's pending operations observed and produced, in call
// order, for the run's final document.
func (t *TaskTable) Tracked(id record.TaskID) record.TaskOperations {
	var tracked record.TaskOperations
	for _, op := range t.Operations(id) {
		tracked.Operations = append(tracked.Operations, record.TaskOperation{Kind: op.Kind, Executed: !op.Carried})
		for _, b := range op.Basis {
			tracked.Basis = append(tracked.Basis, record.TaskBasisEntry{Kind: b.Kind, Key: b.Key})
		}
		for _, out := range op.Outputs {
			tracked.Resources = append(tracked.Resources, record.TaskResource{Kind: out.Kind, Path: out.Path, Fingerprint: out.Digest})
		}
	}
	return tracked
}
