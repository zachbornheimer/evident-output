package engine

import (
	"github.com/zachbornheimer/evident-output/internal/change"
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/record"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// This file is the engine's side of the interfaces change declares (Host):
// what the Output writes and shows on behalf of the changes a Task performs.

// resourceWaitingPrefix opens the live activity a Task shows while one of its
// claims waits on a conflicting holder.
const resourceWaitingPrefix = "waiting for "

// changeHost is the Output, as change asks for it.
type changeHost struct{ out *Output }

// outputOf is the Output a Run's changes report into.
func outputOf(run *change.Run) *Output { return run.Host().(changeHost).out }

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

// ShowWaiting makes id's row read "waiting for <label>" and returns the
// function that restores what it showed before.
func (h changeHost) ShowWaiting(id record.TaskID, label string) (restore func()) {
	o, taskID := h.out, string(id)
	text := txt.Text(resourceWaitingPrefix + label)
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[taskID]
	if st == nil || core.IsTerminalTask(st.node.Rec.State()) {
		return nil
	}
	prior := st.node.Rec.Phase()
	o.setLiveOnlyPhaseLocked(st, text)
	return func() {
		o.mu.Lock()
		defer o.mu.Unlock()
		st := o.taskStates[taskID]
		if st == nil || core.IsTerminalTask(st.node.Rec.State()) || st.node.Rec.Phase() != text {
			return
		}
		o.setLiveOnlyPhaseLocked(st, prior)
	}
}
