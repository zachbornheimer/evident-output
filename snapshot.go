package evo

import "time"

// Snapshot is an immutable complete presentation state at a version.
type Snapshot struct {
	Version     uint64
	OutputID    string
	Subject     string
	Items       []ItemSnapshot
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
}

// ItemSnapshot is an immutable item view.
type ItemSnapshot struct {
	ID          string
	Key         string // optional stable machine key (evo.ID); empty when unset
	Name        string
	State       EntityState
	Problems    []Problem
	Because     string
	Actions     []Action
	Declaration int
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
	Progress   Progress
	Summary    string
	Problems   []Problem
	Actions    []Action
	// Skipped/Kept are the disposition taxonomy accumulated by
	// TaskHandle.Skipped/Kept — the source the "! skipped N (...)" / "!  kept
	// N (...)" render lines derive counts and reason partitions from.
	Skipped     []TaxonomyRecord
	Kept        []TaxonomyRecord
	Collection  string
	Declaration int
}

// TaxonomyRecord is one accumulated (reason, name) disposition entry —
// recorded by TaskHandle.Skipped or TaskHandle.Kept, never assembled by hand.
type TaxonomyRecord struct {
	Reason string
	Name   string
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
}

// PlanSnapshot is an immutable plan section.
type PlanSnapshot struct {
	ID      string
	Subject string
	Records []EffectRecord
}

// EffectRecord is one semantic change or plan row.
type EffectRecord struct {
	Verb     string
	Quantity int64
	HasQty   bool
	Object   string
}
