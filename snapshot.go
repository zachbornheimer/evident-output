package evo

import "time"

// Snapshot is an immutable complete presentation state at a version.
type Snapshot struct {
	Version     uint64
	OutputID    string
	Subject     string
	Tasks       []TaskSnapshot
	Collections []TasksSnapshot
	Changes     []ChangesSnapshot
	Plans       []PlanSnapshot
	Messages    []MessageSnapshot
	// Lines is a derived compatibility projection of projected message texts
	// (and legacy debug history lines). Prefer Messages for structured consumers.
	Lines      []string
	Actions    []Action
	Conclusion *Conclusion
	Timestamp  time.Time
	// DryRun mirrors Config.DryRun: the run's mutation verbs are Plan-only
	// (never Changes). The plain/final projection uses this to open with an
	// unmissable marker line — no caller decides whether to announce it.
	DryRun bool
}

// TaskSnapshot is an immutable task view.
type TaskSnapshot struct {
	ID    string
	Key   string // optional stable machine key (evo.ID); empty when unset
	Name  string
	State EntityState
	Phase string
	// ActivityAt is the domain-clock time of the most recent Phase or Progress
	// call; the live renderer uses it to grow a heartbeat suffix once stale
	// (see phaseStaleAfter). Zero when the task has never had Phase/Progress set.
	ActivityAt time.Time
	// liveFirstSeenAt is presentation-internal bookkeeping (live.go's
	// activitySince) for the universal-heartbeat anchor on a row that has no
	// ActivityAt — never part of the public snapshot contract.
	liveFirstSeenAt time.Time
	Progress        Progress
	Summary         string
	Problems        []Problem
	Actions         []Action
	// Skipped/Kept are the disposition taxonomy accumulated by
	// TaskHandle.Skipped/Kept — the source the "! skipped N (...)" / "!  kept
	// N (...)" render lines derive counts and reason partitions from.
	Skipped     []TaxonomyRecord
	Kept        []TaxonomyRecord
	Collection  string
	Declaration int
	// synthetic marks a task the library invented to carry an output-level
	// outcome (Output.Failf/Cancel) rather than one the caller declared —
	// unexported: it is presentation-internal bookkeeping (coalesce.go),
	// never part of the public snapshot contract.
	synthetic bool
	// unchanged marks a Done task resolved via Task.Unchanged/Unchangedf
	// (I7) — unexported: conclusion-inference-internal bookkeeping, never
	// part of the public snapshot contract.
	unchanged bool
}

// TaxonomyRecord is one accumulated (reason, name) disposition entry —
// recorded by TaskHandle.Skipped or TaskHandle.Kept, never assembled by hand.
type TaxonomyRecord struct {
	Reason string
	Name   string
	// Causes holds the sanitized text of any errs passed to Skipped/Kept for
	// this record — evidence for why the disposition happened, rendered as
	// one bounded └─ line under the count row (first cause + "(+N more)"),
	// full list under Verbose.
	Causes []string
}

// TasksSnapshot is an immutable collection view.
type TasksSnapshot struct {
	ID          string
	Name        string
	State       EntityState
	Summary     string
	Tasks       []TaskSnapshot
	Declaration int
}

// ChangesSnapshot is an immutable changes section.
type ChangesSnapshot struct {
	ID      string
	Subject string
	Records []EffectRecord
	// IntendedVerb is the first mutation verb recorded for this section, even
	// when every record ended up with zero quantity and none survived into
	// Records. Empty when no verb was ever recorded (evo-rec.md "empty effect
	// section grammar"). Never caller-assembled.
	IntendedVerb string
}

// PlanSnapshot is an immutable plan section.
type PlanSnapshot struct {
	ID      string
	Subject string
	Records []EffectRecord
	// IntendedVerb mirrors ChangesSnapshot.IntendedVerb for plan sections.
	IntendedVerb string
}

// EffectRecord is one semantic change or plan row.
type EffectRecord struct {
	Verb     string
	Quantity int64
	HasQty   bool
	Object   string
}
