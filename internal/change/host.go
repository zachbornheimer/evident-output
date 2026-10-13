// This file declares what a Run's changes need of the engine around them.
// The engine implements it; change never imports the engine.

package change

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/freshness"
	sysfs "github.com/zachbornheimer/evident-output/internal/fs"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// Host is the run around one Run's changes: how it was configured, where the
// changes write what they did, what they ask of freshness and what they show
// while they wait. It is assembled from small interfaces so each caller names
// only what it uses.
type Host interface {
	Settings
	Journal
	Manifest
	Activity
	Spawner
}

// Settings is how the Run was configured.
type Settings interface {
	// DryRun reports whether the Run plans its changes instead of making them.
	DryRun() bool
	// FS is the filesystem every File and Patch inspects and changes through.
	FS() sysfs.FS
}

// Journal is where changes write what they did. Every method is keyed by the
// Task the change belongs to: change keeps no per-Task state in the engine.
type Journal interface {
	// Ledger resolves id's ledger section once, before the change runs, so a
	// concurrent resolution of the Task cannot turn a real change into
	// spurious misuse afterwards. It fails when the Run is closed or the
	// Task already resolved; the failure is already recorded as misuse where
	// the cause is not simply that the Task no longer exists.
	Ledger(id record.TaskID) (Ledger, error)
	// AttachVerification adds the per-attribute evidence a reconcile
	// produced to id's record, before the Task resolves.
	AttachVerification(id record.TaskID, details []record.VerificationDetail)
	// Observe narrates one step of a tracked operation.
	Observe(Observation)
	// TaskName is the name id's row shows, or id itself when the Task is
	// unknown.
	TaskName(id record.TaskID) string
	// RecordMisuse reports a program error that belongs to the Run rather
	// than to one Task, so the Run fails even when every Task it names has
	// already settled.
	RecordMisuse(err error)
}

// Ledger is one Task's ledger section, resolved once.
type Ledger interface {
	// Applies reports whether the Run records changes made (true) or changes
	// planned (false).
	Applies() bool
	// Record adds e to the section.
	Record(e record.LedgerEntry)
}

// Manifest is the Run's remembered reconciliation state, as changes consult
// and extend it.
type Manifest interface {
	// Manifest opens, on first use, the Run's manifest store. A miss or an
	// open failure degrades the Run to live-filesystem-only behavior.
	Manifest(ctx context.Context) (*freshness.ManifestStore, error)
	// NoteManifestWarning surfaces a store's safe cache-miss warning once per
	// Run, as a Fact: a miss is always safe to continue past.
	NoteManifestWarning(warning error)
	// TaskManifest is id's stable manifest key and the ordinal of its next
	// operation. ok is false when the Task is unknown.
	TaskManifest(id record.TaskID) (key string, ordinal int, ok bool)
	// AppendOperation records rec as id's next operation, committed only if
	// the Task itself settles Done.
	AppendOperation(id record.TaskID, rec freshness.OperationRecord)
	// PlanOperation holds id's next operation's place without a record: it
	// was found stale and, in a planned Run, left unapplied.
	PlanOperation(id record.TaskID)
}

// Activity is what a Task shows beside its row while a change waits.
type Activity interface {
	// ShowWaiting makes id's row read "waiting for <label>", and returns the
	// function that puts back what the row showed before, unless something
	// else replaced the waiting text in the meantime. It returns nil when
	// there is no live row to show it on.
	ShowWaiting(id record.TaskID, label string) (restore func())
}

// Spawner runs the child processes Exec asks for.
type Spawner interface {
	// Spawn runs cmd for id and waits for it, feeding both output streams
	// to id's capture (sanitized, redacted, bounded) and each completed line
	// to its live activity. The result's Ran is true and its Stdout, Stderr
	// and Truncated come from that same capture whenever the child reached a
	// terminal exit status; a spawn or capture-flush failure returns a zero
	// result and an error naming the executable path. Cancelling ctx kills
	// the child.
	Spawn(ctx context.Context, id record.TaskID, cmd SpawnCommand) (ExecResult, error)
}
