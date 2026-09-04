package core

import "time"

// EventSchemaVersion is the durable event schema version.
// Tracks the 0.3 contract series (pre-1.0 wire format may still evolve).
// Bumped from 0.2 (v0.4.0/P8): the "task.warned" event type (task.go, added
// alongside TaskSnapshot.Warnings in P2) was never reflected in the schema
// version — a machine consumer pinned to 0.2 had no signal a new event type
// existed.
const EventSchemaVersion = "0.3"

// Event is an immutable journal record.
type Event struct {
	SchemaVersion string
	Sequence      uint64
	Timestamp     time.Time
	Type          string
	OutputID      string
	EntityID      string
	Name          string
	State         string
	Completed     *int64
	Total         *int64
	Activation    string
	Payload       map[string]any
}
