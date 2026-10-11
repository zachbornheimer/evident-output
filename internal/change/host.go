// This file declares what a Run's changes need of the engine around them.
// The engine implements it; change never imports the engine.

package change

import "github.com/zachbornheimer/evident-output/internal/record"

// Host is the run around one Run's changes: where they write what they did
// and what they show while they wait. It is assembled from small interfaces
// so each caller names only what it uses.
type Host interface {
	Journal
	Activity
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
}

// Ledger is one Task's ledger section, resolved once.
type Ledger interface {
	// Applies reports whether the Run records changes made (true) or changes
	// planned (false).
	Applies() bool
	// Record adds e to the section.
	Record(e record.LedgerEntry)
}

// Activity is what a Task shows beside its row while a change waits.
type Activity interface {
	// ShowWaiting makes id's row read "waiting for <label>", and returns the
	// function that puts back what the row showed before, unless something
	// else replaced the waiting text in the meantime. It returns nil when
	// there is no live row to show it on.
	ShowWaiting(id record.TaskID, label string) (restore func())
}
