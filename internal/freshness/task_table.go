// This file owns what freshness remembers about each Task during a Run: the
// inputs it declared, the Basis it observed, the operation records it will
// commit, and the Verify checks it registered.

package freshness

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/zachbornheimer/evident-output/internal/record"
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

// BasisSource is one Task Basis input: something whose content identity is
// observed when the Task starts. Build one with FingerprintBasis.
type BasisSource interface {
	observe(ctx context.Context) (BasisRecord, error)
}

// FingerprintBasis observes a Fingerprint (FSPath, Value, App).
func FingerprintBasis(f Fingerprint) BasisSource { return fingerprintBasis{inner: f} }

type fingerprintBasis struct{ inner Fingerprint }

func (b fingerprintBasis) observe(ctx context.Context) (BasisRecord, error) {
	records, err := basisRecordsFrom(ctx, []Fingerprint{b.inner})
	if err != nil {
		return BasisRecord{}, err
	}
	return records[0], nil
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
	// ManifestStore.Operation's ordinal indexes into. Never populated during
	// dry-run, which commits nothing (§8.2).
	operations []OperationRecord
	// verifiers are the Verify checks, ANDed in registration order (§9.1).
	verifiers []Verifier
}

// TaskTable remembers freshness state per Task, keyed by record.TaskID. It
// is safe for concurrent use.
type TaskTable struct {
	mu    sync.Mutex
	tasks map[record.TaskID]*taskEntry
}

// NewTaskTable returns an empty table.
func NewTaskTable() *TaskTable {
	return &TaskTable{tasks: make(map[record.TaskID]*taskEntry)}
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

// OperationCount is how many operations id has recorded: the ordinal its next
// operation takes within its manifest record.
func (t *TaskTable) OperationCount(id record.TaskID) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if entry := t.tasks[id]; entry != nil {
		return len(entry.operations)
	}
	return 0
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

// BasisJudgement is the verdict of judging a Task's declared Basis.
type BasisJudgement struct {
	// Declared is false when the Task declared no Basis; nothing else is set.
	Declared bool
	// Observed is the Basis as observed now, canonicalized.
	Observed []BasisRecord
	// Current is true when the Task's last successful Run recorded the same
	// identity and its recorded outputs still hold.
	Current bool
}

// JudgeBasis observes id's Basis as the Task starts and reports whether the
// Task's last successful Run recorded the same identity. The observation
// holds no resource claim, so it never orders or blocks any other Task. A
// Task with no Basis is never current. Without a prior record the Task runs.
// A current Task adopts its prior operations as its own pending ones, so a
// later commit rewrites identical state.
func (t *TaskTable) JudgeBasis(ctx context.Context, session *ManifestSession, task TaskView) (BasisJudgement, error) {
	id := task.TaskID()
	t.mu.Lock()
	var inputs []BasisSource
	if entry := t.tasks[id]; entry != nil {
		inputs = slices.Clone(entry.inputs)
	}
	t.mu.Unlock()
	if len(inputs) == 0 {
		return BasisJudgement{}, nil
	}
	observed, err := observeTaskBasis(ctx, inputs)
	if err != nil {
		return BasisJudgement{}, err
	}
	store, err := session.Store(ctx)
	if err != nil {
		return BasisJudgement{}, fmt.Errorf("evo: Basis: %w", err)
	}
	prior, ok := store.Task(task.ManifestKey())
	operations, priorBasis := splitBasisOperation(prior.Operations)
	current := ok && priorBasis != nil && BasisRecordsEqual(priorBasis, observed)
	if current {
		if current, err = RecordedOutputsHold(ctx, operations); err != nil {
			return BasisJudgement{}, err
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := t.entryLocked(id)
	entry.observed = observed
	if current {
		entry.operations = operations
	}
	return BasisJudgement{Declared: true, Observed: observed, Current: current}, nil
}

// observeTaskBasis observes every input and canonicalizes the result. Basis
// order is not identity; a repeated (kind, key) is a programmer error.
func observeTaskBasis(ctx context.Context, inputs []BasisSource) ([]BasisRecord, error) {
	records := make([]BasisRecord, 0, len(inputs))
	for _, in := range inputs {
		rec, err := in.observe(ctx)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return canonicalBasis(records)
}

// splitBasisOperation separates a committed task's trailing Basis record
// from its other operations. basis is nil when the task recorded none.
func splitBasisOperation(ops []OperationRecord) (rest []OperationRecord, basis []BasisRecord) {
	if n := len(ops); n > 0 && ops[n-1].Kind == TaskBasisOperationKind {
		return slices.Clone(ops[:n-1]), ops[n-1].Basis
	}
	return slices.Clone(ops), nil
}

// operationsToCommitLocked is id's operations plus, when it declared a Basis, the
// Basis record last. Callers hold t.mu.
func (t *TaskTable) operationsToCommitLocked(id record.TaskID) []OperationRecord {
	entry := t.tasks[id]
	if entry == nil {
		return nil
	}
	ops := slices.Clone(entry.operations)
	if entry.observed != nil {
		ops = append(ops, OperationRecord{Kind: TaskBasisOperationKind, Basis: entry.observed})
	}
	return ops
}

// TaskCommit is what committing one Task's manifest record did.
type TaskCommit struct {
	// Written is true when the record was handed to the store's writer.
	Written bool
	// Operations is how many operations the written record holds.
	Operations int
	// Err is the failure to hand the record over, if any.
	Err error
}

// Commit persists task's accumulated operations as this Run's truth (spec
// §11.3) once the Task has settled Done. A Run with no opened manifest
// commits nothing (no Task in this Run ever used File/Exec/Patch, so
// nothing opened it), keeping a purely opaque consumer's Run free of any
// manifest file at all.
//
// A Task with tracked operations commits its precise provenance at once. A
// Task with none is opaque: its record carries the application fingerprint
// as its DefinitionFingerprint (ZYS-817 Decisions 2026-09-23), never folded
// into any operation's Basis. Nothing reads that record back within the Run,
// so it is staged and written with the next commit or at Save instead of
// costing each settling Task a full manifest rewrite and fsync.
func (t *TaskTable) Commit(ctx context.Context, session *ManifestSession, task TaskView) TaskCommit {
	store := session.Opened()
	if store == nil {
		return TaskCommit{}
	}
	app := session.Application()
	key := task.ManifestKey()
	t.mu.Lock()
	ops := t.operationsToCommitLocked(task.TaskID())
	t.mu.Unlock()
	if len(ops) == 0 {
		store.StageTask(app, TaskRecord{
			Key:                   key,
			DefinitionFingerprint: OpaqueTaskDefinitionFingerprint(key, app.Fingerprint),
		})
		return TaskCommit{}
	}
	if err := store.CommitTask(ctx, app, TaskRecord{Key: key, Operations: ops}); err != nil {
		return TaskCommit{Err: err}
	}
	return TaskCommit{Written: true, Operations: len(ops)}
}
