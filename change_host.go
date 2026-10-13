package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/change"
	"github.com/zachbornheimer/evident-output/internal/freshness"
	txt "github.com/zachbornheimer/evident-output/internal/project"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// This file is the engine's side of the interfaces change declares (Host):
// what the Output writes, remembers and shows on behalf of the changes a
// Task performs.

// resourceWaitingPrefix opens the live activity a Task shows while one of its
// claims waits on a conflicting holder.
const resourceWaitingPrefix = "waiting for "

// changeHost is the Output, as change asks for it.
type changeHost struct{ out *Output }

// DryRun reports whether this Output is configured for dry-run/preview
// tense — the same flag evo.Effect and evo.File render by.
func (o *Output) DryRun() bool {
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.cfg.dryRun
}

// DryRun reports whether the Run plans its changes instead of making them.
func (h changeHost) DryRun() bool { return h.out.DryRun() }

// FS is the filesystem File performs I/O through — real os calls in
// ordinary use, a scripted fake under test (facade rule).
func (h changeHost) FS() FileFS {
	h.out.mu.Lock()
	defer h.out.mu.Unlock()
	return h.out.cfg.fileFS
}

// taskLedger is one Task's ledger section, resolved once.
type taskLedger struct {
	out    *Output
	taskID string
	target ledgerTarget
}

// Applies reports whether the ledger records changes made, not planned.
func (l taskLedger) Applies() bool { return l.target.tense == tenseChanged }

// Record adds e to the Task's section.
func (l taskLedger) Record(e record.LedgerEntry) {
	l.out.recordResolvedEntry(l.taskID, l.target, e)
}

// Ledger resolves id's ledger section; see Output.resolveLedgerTarget.
func (h changeHost) Ledger(id record.TaskID) (change.Ledger, error) {
	target, err := h.out.resolveLedgerTarget(string(id))
	if err != nil {
		return nil, err
	}
	return taskLedger{out: h.out, taskID: string(id), target: target}, nil
}

// AttachVerification adds the per-attribute evidence a reconcile produced to
// id's record; see Output.attachVerificationLocked.
func (h changeHost) AttachVerification(id record.TaskID, details []record.VerificationDetail) {
	h.out.mu.Lock()
	defer h.out.mu.Unlock()
	h.out.attachVerificationLocked(string(id), details)
}

// Manifest opens this Run's manifest on first use; see Output.manifestFor.
func (h changeHost) Manifest(ctx context.Context) (*freshness.ManifestStore, error) {
	return h.out.manifestFor(ctx)
}

// NoteManifestWarning surfaces a safe cache-miss warning once per Run.
func (h changeHost) NoteManifestWarning(warning error) { h.out.emitManifestWarningOnce(warning) }

// TaskManifest is id's stable manifest key and its next operation's ordinal.
func (h changeHost) TaskManifest(id record.TaskID) (key string, ordinal int, ok bool) {
	h.out.mu.Lock()
	defer h.out.mu.Unlock()
	return h.out.taskManifestKeyLocked(string(id))
}

// AppendOperation records rec as id's next pending operation.
func (h changeHost) AppendOperation(id record.TaskID, rec freshness.OperationRecord) {
	h.out.mu.Lock()
	defer h.out.mu.Unlock()
	h.out.appendManifestOperationLocked(string(id), rec)
}

// PlanOperation holds id's next operation's place without a record.
func (h changeHost) PlanOperation(id record.TaskID) {
	h.out.mu.Lock()
	defer h.out.mu.Unlock()
	h.out.planManifestOperationLocked(string(id))
}

// ShowWaiting makes id's row read "waiting for <label>" and returns the
// function that restores what it showed before.
func (h changeHost) ShowWaiting(id record.TaskID, label string) (restore func()) {
	o, taskID := h.out, string(id)
	text := txt.Text(resourceWaitingPrefix + label)
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[taskID]
	if st == nil || record.IsTerminalTask(st.node.Rec.State()) {
		return nil
	}
	prior := st.node.Rec.Phase()
	o.setLiveOnlyPhaseLocked(st, text)
	return func() {
		o.mu.Lock()
		defer o.mu.Unlock()
		st := o.taskStates[taskID]
		if st == nil || record.IsTerminalTask(st.node.Rec.State()) || st.node.Rec.Phase() != text {
			return
		}
		o.setLiveOnlyPhaseLocked(st, prior)
	}
}
