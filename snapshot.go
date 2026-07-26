package evo

import "time"

// Snapshot is an immutable complete presentation state at a version.
type Snapshot struct {
	Version     uint64
	Subject     string
	Items       []ItemSnapshot
	Tasks       []TaskSnapshot
	Collections []TasksSnapshot
	Changes     []ChangesSnapshot
	Plans       []PlanSnapshot
	Lines       []string
	Actions     []Action
	Conclusion  *Conclusion
	Timestamp   time.Time
}

// ItemSnapshot is an immutable item view.
type ItemSnapshot struct {
	ID          string
	Name        string
	State       EntityState
	Problems    []Problem
	Because     string
	Actions     []Action
	Declaration int
}

// TaskSnapshot is an immutable task view.
type TaskSnapshot struct {
	ID          string
	Name        string
	State       EntityState
	Phase       string
	Progress    Progress
	Summary     string
	Problems    []Problem
	Actions     []Action
	Collection  string
	Declaration int
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
