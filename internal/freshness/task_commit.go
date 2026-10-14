// This file commits a settled Task's record to the manifest.

package freshness

import (
	"context"
	"slices"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

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
// manifest file at all. A planned Run commits nothing either: it applied
// nothing, and a record it wrote would make the next real Run skip or repeat
// work wrongly.
//
// A Task with tracked operations commits its precise provenance at once. A
// Task with none is opaque: its record carries the application fingerprint
// as its DefinitionFingerprint (ZYS-817 Decisions 2026-09-23), never folded
// into any operation's Basis. Nothing reads that record back within the Run,
// so it is staged and written with the next commit or at Save instead of
// costing each settling Task a full manifest rewrite and fsync.
func (t *TaskTable) Commit(ctx context.Context, session *ManifestSession, task TaskView, mode RunMode) TaskCommit {
	store := session.Opened()
	if store == nil || !mode.Remembers() {
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

// Retract forgets task's committed record, for a Task that settled Done and
// was then overturned to Failed. Like Commit, a Run that opened no manifest
// or only plans retracts nothing: it committed nothing to take back.
func (t *TaskTable) Retract(session *ManifestSession, task TaskView, mode RunMode) {
	store := session.Opened()
	if store == nil || !mode.Remembers() {
		return
	}
	store.RetractTask(task.ManifestKey())
}
