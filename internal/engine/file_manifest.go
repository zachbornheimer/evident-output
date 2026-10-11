package engine

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/freshness"
	"github.com/zachbornheimer/evident-output/internal/record"
	txt "github.com/zachbornheimer/evident-output/internal/text"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// emitManifestWarningOnce surfaces a manifest Store's safe cache-miss
// warning (corrupt/unknown prior manifest, spec §11.3) as a run-scoped Fact
// exactly once per Run — never as an error, since a miss is always safe to
// continue past.
func (o *Output) emitManifestWarningOnce(w error) {
	if w == nil || !o.manifest.FirstMissWarning() {
		return
	}
	o.Fact("manifest", w.Error())
}

// taskManifestKeyLocked returns taskID's stable manifest Task key (§3.1's
// stableKey, already computed at declaration) and the next pending
// operation ordinal within it. Callers must already hold o.mu.
func (o *Output) taskManifestKeyLocked(taskID string) (key string, ordinal int, ok bool) {
	st := o.taskStates[taskID]
	if st == nil {
		return "", 0, false
	}
	return st.ManifestKey(), o.taskFreshness.OperationCount(st.TaskID()), true
}

// appendManifestOperationLocked records rec as taskID's next pending
// operation, committed only if/when the Task itself settles Done (spec
// §8.2/§11.3). Callers must already hold o.mu.
func (o *Output) appendManifestOperationLocked(taskID string, rec freshness.OperationRecord) {
	if st := o.taskStates[taskID]; st != nil {
		o.taskFreshness.AppendOperation(st.TaskID(), rec)
	}
}

// planManifestOperationLocked notes that taskID's next operation was found
// stale and, in a planned Run, left unapplied; it holds its place in the
// Task's order without a record. Callers must already hold o.mu.
func (o *Output) planManifestOperationLocked(taskID string) {
	if st := o.taskStates[taskID]; st != nil {
		o.taskFreshness.PlanOperation(st.TaskID())
	}
}

// commitManifestTaskLocked persists taskID's accumulated operations as this
// Run's truth (spec §11.3) once the Task has settled Done; see
// freshness.TaskTable.Commit. Callers must already hold o.mu.
func (o *Output) commitManifestTaskLocked(ctx context.Context, taskID string) {
	st := o.taskStates[taskID]
	if st == nil {
		return
	}
	commit := o.taskFreshness.Commit(ctx, o.manifest, st, o.runModeLocked())
	if commit.Err != nil {
		o.warnManifestUnsavedLocked(commit.Err)
		return
	}
	if commit.Written {
		o.emitWireEventLocked(wire.EventManifestTaskCommitted, taskID, map[string]any{
			"operations": commit.Operations,
		})
	}
}

// saveManifest waits until every record this Run committed or staged is
// on disk (see freshness.ManifestSession.Save) and warns on the run when it
// could not be: the next run re-executes work this one did, and the reader
// must know why.
func (o *Output) saveManifest() {
	if err := o.manifest.Save(); err != nil {
		o.mu.Lock()
		defer o.mu.Unlock()
		if !o.finished {
			o.warnManifestUnsavedLocked(err)
		}
	}
}

// warnManifestUnsavedLocked states once per run that the manifest could
// not be saved. Callers must already hold o.mu.
func (o *Output) warnManifestUnsavedLocked(err error) {
	if !o.manifest.FirstUnsavedWarning() {
		return
	}
	o.warnLocked(record.ApplyProblemOptions(txt.Text("manifest not saved: "+err.Error()), nil))
}
